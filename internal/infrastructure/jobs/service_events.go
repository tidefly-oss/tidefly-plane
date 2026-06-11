package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// WatchContainerEvents is the single subscriber to the runtime EventStream.
// It handles three concerns in one place:
//   - AppLog writes for all lifecycle events
//   - In-app Notifications for stop/die/kill/oom
//   - External Notifier (Slack/Discord/email) for oom + failed heal
//   - Self-heal enqueue for die/kill/oom
//
// The EventBus (WebSocket) gets its updates via handler.bus.Publish inside
// handleContainerEvent, so StartRuntimeWatcher in eventbus/bus.go should be
// removed once this is confirmed working.
func (s *Server) WatchContainerEvents(ctx context.Context) {
	s.log.Info("jobs", "container event watcher started")
	for {
		if err := s.watchOnce(ctx); err != nil {
			if ctx.Err() != nil {
				s.log.Info("jobs", "container event watcher stopped")
				return
			}
			s.log.Warnw("jobs", "event watcher disconnected, reconnecting in 5s", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (s *Server) watchOnce(ctx context.Context) error {
	eventCh, errCh := s.handler.rt.EventStream(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case event, ok := <-eventCh:
			if !ok {
				return fmt.Errorf("event channel closed")
			}
			s.handleContainerEvent(event)
		}
	}
}

// handleContainerEvent processes a single container lifecycle event.
// Severity matrix:
//
//	create / start / restart / stop / destroy / pause / unpause → AppLog INFO/WARN, no external notify
//	die / kill → AppLog WARN, in-app Notification WARN (if NotifyOnContainerDown), self-heal enqueue
//	oom        → AppLog ERROR, in-app Notification ERROR, external Notifier, self-heal enqueue
func (s *Server) handleContainerEvent(event runtime.ContainerEvent) {
	if event.Name == "" {
		return
	}

	// Skip Blue-Green slot teardown events — expected churn during deploy.
	if event.Labels["tidefly.slot"] != "" &&
		(event.Type == runtime.EventStop || event.Type == runtime.EventDie || event.Type == runtime.EventDestroy) {
		return
	}

	switch event.Type {

	// ── Informational lifecycle ───────────────────────────────────────────────
	case runtime.EventCreate:
		s.handler.log.ContainerEvent("INFO", event.ContainerID, event.Name,
			fmt.Sprintf("container created (image: %s)", event.Image), "")

	case runtime.EventStart:
		s.handler.log.ContainerEvent("INFO", event.ContainerID, event.Name,
			"container started", "")

	case runtime.EventRestart:
		s.handler.log.ContainerEvent("INFO", event.ContainerID, event.Name,
			"container restarted", "")

	case runtime.EventUnpause:
		s.handler.log.ContainerEvent("INFO", event.ContainerID, event.Name,
			"container unpaused", "")

	case runtime.EventDestroy:
		s.handler.log.ContainerEvent("INFO", event.ContainerID, event.Name,
			"container removed", "")

	case runtime.EventPause:
		s.handler.log.ContainerEvent("WARN", event.ContainerID, event.Name,
			"container paused", "")

	// ── Degraded — warn + conditional in-app notification ────────────────────
	case runtime.EventStop:
		s.handler.log.ContainerEvent("WARN", event.ContainerID, event.Name,
			"container stopped", "")
		s.maybeNotify(event, models.SeverityWarn,
			fmt.Sprintf("container %q stopped", event.Name))

	case runtime.EventDie:
		s.handler.log.ContainerEvent("WARN", event.ContainerID, event.Name,
			"container exited unexpectedly — self-healing queued", "")
		s.maybeNotify(event, models.SeverityWarn,
			fmt.Sprintf("container %q exited unexpectedly — attempting recovery", event.Name))
		s.enqueueHeal(event)

	case runtime.EventKill:
		s.handler.log.ContainerEvent("WARN", event.ContainerID, event.Name,
			fmt.Sprintf("container killed (signal: %s) — self-healing queued", event.Labels["signal"]), "")
		s.maybeNotify(event, models.SeverityWarn,
			fmt.Sprintf("container %q was killed (signal: %s) — attempting recovery",
				event.Name, event.Labels["signal"]))
		s.enqueueHeal(event)

	// ── Critical — error + external notify + heal ─────────────────────────────
	case runtime.EventOOM:
		s.handler.log.ContainerEvent("ERROR", event.ContainerID, event.Name,
			"container out of memory — self-healing queued", "")
		s.notifyOOM(event)
		s.enqueueHeal(event)
	}
}

// maybeNotify upserts an in-app notification only if NotifyOnContainerDown is
// enabled in system settings. Skips the DB lookup if notifSvc is nil.
func (s *Server) maybeNotify(event runtime.ContainerEvent, severity models.NotificationSeverity, msg string) {
	if s.handler.notifSvc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var settings models.SystemSettings
		if err := s.handler.db.WithContext(ctx).First(&settings).Error; err != nil {
			return
		}
		if !settings.NotifyOnContainerDown {
			return
		}
		_ = s.handler.notifSvc.Upsert(ctx, event.ContainerID, event.Name, severity, msg)
	}()
}

// notifyOOM always creates an in-app notification AND sends to external channels
// (Slack/Discord/email) — OOM is always actionable regardless of settings.
func (s *Server) notifyOOM(event runtime.ContainerEvent) {
	msg := fmt.Sprintf("container %q ran out of memory — attempting recovery", event.Name)

	if s.handler.notifSvc != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.handler.notifSvc.Upsert(ctx, event.ContainerID, event.Name, models.SeverityError, msg)
		}()
	}

	if s.svcHandler.notifier != nil {
		go func() {
			s.svcHandler.notifier.Send(context.Background(), notification.Event{
				Title:   fmt.Sprintf("[OOM] %s", event.Name),
				Message: msg,
				Level:   "error",
			})
		}()
	}
}

// enqueueHeal enqueues an immediate self-heal task for the affected service.
// Blue-Green containers have their slot suffix stripped to find the service name.
func (s *Server) enqueueHeal(event runtime.ContainerEvent) {
	serviceName := deriveServiceName(event.Name)
	s.log.Info("jobs", fmt.Sprintf(
		"event watcher: %s on %q (service=%q) — enqueuing heal",
		event.Type, event.Name, serviceName,
	))
	if err := EnqueueServiceHeal(s.client, serviceName, event.ContainerID, string(event.Type)); err != nil {
		s.log.Warnw("jobs", "failed to enqueue heal", "service", serviceName, "err", err)
	}
}

// deriveServiceName strips blue/green slot suffixes from container names.
// "myservice-green" → "myservice", "myservice" → "myservice"
func deriveServiceName(containerName string) string {
	for _, suffix := range []string{"-green", "-blue"} {
		if strings.HasSuffix(containerName, suffix) {
			return strings.TrimSuffix(containerName, suffix)
		}
	}
	return containerName
}
