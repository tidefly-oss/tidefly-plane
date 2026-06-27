package jobs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
)

// recentStops tracks containers that received a stop event recently
// so we can distinguish manual stops from unexpected exits in EventDie.
type stopTracker struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newStopTracker() *stopTracker {
	return &stopTracker{seen: make(map[string]time.Time)}
}

func (t *stopTracker) record(containerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seen[containerID] = time.Now()
}

func (t *stopTracker) wasRecentlyStopped(containerID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	ts, ok := t.seen[containerID]
	if !ok {
		return false
	}
	delete(t.seen, containerID)
	return time.Since(ts) < 3*time.Second
}

func (s *Server) watchContainerEvents(ctx context.Context) {
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
	eventCh, errCh := s.system.rt.EventStream(ctx)
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
			s.handleContainerEvent(ctx, event)
		}
	}
}

func (s *Server) handleContainerEvent(ctx context.Context, event runtime.ContainerEvent) {
	if event.Name == "" {
		return
	}

	// Skip expected churn from blue-green slot teardown
	if event.Labels["tidefly.slot"] != "" &&
		(event.Type == runtime.EventStop || event.Type == runtime.EventDie || event.Type == runtime.EventDestroy) {
		return
	}

	switch event.Type {
	case runtime.EventCreate:
		s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, fmt.Sprintf("container created (image: %s)", event.Image), "")

	case runtime.EventStart:
		s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, "container started", "")

	case runtime.EventRestart:
		s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, "container restarted", "")

	case runtime.EventUnpause:
		s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, "container unpaused", "")

	case runtime.EventDestroy:
		s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, "container removed", "")

	case runtime.EventPause:
		s.system.log.ContainerEvent("WARN", event.ContainerID, event.Name, "container paused", "")

	case runtime.EventStop:
		s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, "container stopped", "")
		s.stops.record(event.ContainerID)

	case runtime.EventKill:
		sig := event.Labels["signal"]
		if sig == "15" || sig == "SIGTERM" {
			s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, fmt.Sprintf("container stopped gracefully (signal: %s)", sig), "")
			s.stops.record(event.ContainerID)
			return
		}
		s.system.log.ContainerEvent("WARN", event.ContainerID, event.Name, fmt.Sprintf("container killed (signal: %s) — self-healing queued", sig), "")
		s.maybeNotify(ctx, event, models.SeverityWarn, fmt.Sprintf("container %q killed (signal: %s) — attempting recovery", event.Name, sig))
		s.enqueueHeal(ctx, event)

	case runtime.EventDie:
		exitCode := event.Labels["exitCode"]
		if exitCode == "0" || s.stops.wasRecentlyStopped(event.ContainerID) {
			s.system.log.ContainerEvent("INFO", event.ContainerID, event.Name, fmt.Sprintf("container exited (exitCode=%s)", exitCode), "")
			return
		}
		s.system.log.ContainerEvent("WARN", event.ContainerID, event.Name, "container exited unexpectedly — self-healing queued", "")
		s.maybeNotify(ctx, event, models.SeverityWarn, fmt.Sprintf("container %q exited — attempting recovery", event.Name))
		s.enqueueHeal(ctx, event)

	case runtime.EventOOM:
		s.system.log.ContainerEvent("ERROR", event.ContainerID, event.Name, "container OOM — self-healing queued", "")
		s.notifyOOM(ctx, event)
		s.enqueueHeal(ctx, event)
	}
}

func (s *Server) maybeNotify(ctx context.Context, event runtime.ContainerEvent, severity models.NotificationSeverity, msg string) {
	if s.system.notifSvc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var settings models.SystemSettings
		if err := s.system.db.WithContext(ctx).First(&settings).Error; err != nil || !settings.NotifyOnContainerDown {
			return
		}
		_ = s.system.notifSvc.Upsert(ctx, event.ContainerID, event.Name, severity, msg)
		if s.svc.notifier != nil && settings.ExternalNotificationsEnabled {
			level := "warning"
			if severity == models.SeverityError || severity == models.SeverityFatal {
				level = "error"
			}
			s.svc.notifier.Send(ctx, notification.Event{
				Title:   fmt.Sprintf("[%s] %s", strings.ToUpper(string(severity)), event.Name),
				Message: msg,
				Level:   level,
			})
		}
	}()
}

func (s *Server) notifyOOM(ctx context.Context, event runtime.ContainerEvent) {
	msg := fmt.Sprintf("container %q ran out of memory — attempting recovery", event.Name)
	if s.system.notifSvc != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.system.notifSvc.Upsert(ctx, event.ContainerID, event.Name, models.SeverityError, msg)
		}()
	}
	if s.svc.notifier != nil {
		go func() {
			s.svc.notifier.Send(context.Background(), notification.Event{
				Title:   fmt.Sprintf("[OOM] %s", event.Name),
				Message: msg,
				Level:   "error",
			})
		}()
	}
}

func (s *Server) enqueueHeal(ctx context.Context, event runtime.ContainerEvent) {
	serviceName := deriveServiceName(event.Name)
	s.log.Info("jobs", fmt.Sprintf("event watcher: %s on %q → enqueuing heal", event.Type, event.Name))

	healCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.river.Insert(healCtx, HealArgs{
		ServiceName: serviceName,
		ContainerID: event.ContainerID,
		Reason:      string(event.Type),
	}, &river.InsertOpts{
		Queue:    "critical",
		Priority: 1,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
		},
	})
	if err != nil {
		s.log.Warnw("jobs", "failed to enqueue heal", "service", serviceName, "err", err)
	}
}

func deriveServiceName(containerName string) string {
	for _, suffix := range []string{"-green", "-blue"} {
		if before, ok := strings.CutSuffix(containerName, suffix); ok {
			return before
		}
	}
	return containerName
}

type WebhookArgs struct {
	WebhookID  string `json:"webhook_id"`
	DeliveryID string `json:"delivery_id"`
	Payload    struct {
		Branch string `json:"branch"`
		Tag    string `json:"tag"`
		Commit string `json:"commit"`
	} `json:"payload"`
}

func (WebhookArgs) Kind() string { return "webhook:deploy" }

func EnqueueWebhook(ctx context.Context, client *river.Client[pgx.Tx], webhookID, deliveryID, branch, tag, commit string) error {
	args := WebhookArgs{WebhookID: webhookID, DeliveryID: deliveryID}
	args.Payload.Branch = branch
	args.Payload.Tag = tag
	args.Payload.Commit = commit
	_, err := client.Insert(ctx, args, &river.InsertOpts{Queue: river.QueueDefault})
	return err
}

func EnqueueDeploy(ctx context.Context, client *river.Client[pgx.Tx], args DeployArgs) error {
	_, err := client.Insert(ctx, args, &river.InsertOpts{Queue: "critical"})
	return err
}

func EnqueueRedeploy(ctx context.Context, client *river.Client[pgx.Tx], serviceID, imageOverride, gitToken string) error {
	_, err := client.Insert(ctx, RedeployArgs{
		ServiceID:     serviceID,
		ImageOverride: imageOverride,
		GitToken:      gitToken,
	}, &river.InsertOpts{Queue: "critical"})
	return err
}

func EnqueueDelete(ctx context.Context, client *river.Client[pgx.Tx], serviceID string) error {
	_, err := client.Insert(ctx, DeleteArgs{ServiceID: serviceID}, &river.InsertOpts{Queue: river.QueueDefault})
	return err
}

func EnqueueCleanup(ctx context.Context, client *river.Client[pgx.Tx], serviceName string, images, volumes []string) error {
	_, err := client.Insert(ctx, CleanupArgs{ServiceName: serviceName, Images: images, Volumes: volumes},
		&river.InsertOpts{Queue: "low"})
	return err
}
