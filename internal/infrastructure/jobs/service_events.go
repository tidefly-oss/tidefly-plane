package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

const TaskServiceHeal = "service:heal"

// ServiceHealPayload carries the info needed to heal a single service.
type ServiceHealPayload struct {
	ServiceName string `json:"service_name"`
	ContainerID string `json:"container_id"`
	Reason      string `json:"reason"` // "die" | "oom" | "kill"
}

// EnqueueServiceHeal enqueues an immediate heal task for a single service.
// Uses TaskID for deduplication — if a heal is already queued, the second is dropped.
func EnqueueServiceHeal(client *asynq.Client, serviceName, containerID, reason string) error {
	data, err := json.Marshal(ServiceHealPayload{
		ServiceName: serviceName,
		ContainerID: containerID,
		Reason:      reason,
	})
	if err != nil {
		return err
	}
	_, err = client.Enqueue(
		asynq.NewTask(TaskServiceHeal, data,
			asynq.MaxRetry(2),
			asynq.Timeout(2*time.Minute),
			asynq.Queue("critical"),
			asynq.TaskID(fmt.Sprintf("heal:%s", serviceName)),
		),
	)
	// TaskID conflict means a heal is already queued — that's fine
	if err != nil && strings.Contains(err.Error(), "task ID already exists") {
		return nil
	}
	return err
}

// WatchContainerEvents subscribes to the runtime event stream and enqueues
// a heal task immediately when a managed service container dies or OOMs.
// Runs as a long-lived goroutine. Reconnects automatically on error.
func (s *Server) WatchContainerEvents(ctx context.Context) {
	s.log.Info("jobs", "container event watcher started")
	for {
		if err := s.watchOnce(ctx); err != nil {
			if ctx.Err() != nil {
				s.log.Info("jobs", "container event watcher stopped")
				return
			}
			s.log.Warnw("jobs", "event watcher error, reconnecting in 5s", "err", err)
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

func (s *Server) handleContainerEvent(event runtime.ContainerEvent) {
	switch event.Type {
	case runtime.EventDie, runtime.EventOOM, runtime.EventKill:
		if event.Name == "" {
			return
		}

		// Derive service name — blue/green containers are named "svc-blue" / "svc-green"
		serviceName := deriveServiceName(event.Name)

		if s.handler.notifSvc != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				// Check settings before notifying
				var settings models.SystemSettings
				if err := s.handler.db.WithContext(ctx).First(&settings).Error; err == nil &&
					settings.NotifyOnContainerDown {
					msg := fmt.Sprintf("container %q stopped (reason: %s)", event.Name, event.Type)
					_ = s.handler.notifSvc.Upsert(ctx, event.ContainerID, event.Name, models.SeverityError, msg)
				}
			}()
		}

		s.log.Info("jobs", fmt.Sprintf(
			"event watcher: %s on %q (service=%q) — enqueuing heal",
			event.Type, event.Name, serviceName,
		))

		if err := EnqueueServiceHeal(s.client, serviceName, event.ContainerID, string(event.Type)); err != nil {
			s.log.Warnw("jobs", "failed to enqueue heal task",
				"service", serviceName, "err", err)
		}
	}
}

// deriveServiceName strips blue/green slot suffixes.
// "myservice-green" → "myservice", "myservice" → "myservice"
func deriveServiceName(containerName string) string {
	for _, suffix := range []string{"-green", "-blue"} {
		if strings.HasSuffix(containerName, suffix) {
			return strings.TrimSuffix(containerName, suffix)
		}
	}
	return containerName
}
