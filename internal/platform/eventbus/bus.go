package eventbus

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/olahol/melody"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

// Bus is the central WebSocket event broadcaster.
type Bus struct {
	m  *melody.Melody
	mu sync.RWMutex
}

func New() *Bus {
	m := melody.New()
	m.Config.MaxMessageSize = 4096
	return &Bus{m: m}
}

// Melody exposes the underlying melody instance for route registration.
func (b *Bus) Melody() *melody.Melody {
	return b.m
}

// Publish broadcasts an event to all sessions subscribed to the event's topic.
func (b *Bus) Publish(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_ = b.m.BroadcastFilter(data, func(s *melody.Session) bool {
		raw, exists := s.Get("topics")
		if !exists {
			return true
		}
		topics, ok := raw.(map[string]bool)
		if !ok {
			return true
		}
		b.mu.RLock()
		defer b.mu.RUnlock()
		return topics[event.Topic] || topics["*"]
	})
}

// SetTopics stores the subscribed topics on a session.
func (b *Bus) SetTopics(s *melody.Session, topics []string) {
	m := make(map[string]bool, len(topics))
	for _, t := range topics {
		m[t] = true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s.Set("topics", m)
}

// StartRuntimeWatcher subscribes to the runtime EventStream and publishes
// container state changes to all connected WS clients.
func (b *Bus) StartRuntimeWatcher(ctx context.Context, rt runtime.Runtime, log *applogger.Logger) {
	go func() {
		for {
			eventCh, errCh := rt.EventStream(ctx)
			for {
				select {
				case <-ctx.Done():
					return
				case err := <-errCh:
					if err != nil {
						log.Warnw("eventbus", "runtime event stream error", "err", err)
					}
					// retry after short delay
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
					}
					goto reconnect
				case evt, ok := <-eventCh:
					if !ok {
						goto reconnect
					}
					b.Publish(Event{
						Type:  EventContainerUpdated,
						Topic: TopicContainers,
						Payload: ContainerUpdatedPayload{
							ID:     evt.ContainerID,
							Name:   evt.Name,
							Status: string(evt.Status),
							State:  string(evt.Status),
						},
					})
				}
			}
		reconnect:
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

// StartMetricsTicker pushes system metrics to all connected WS clients every 10s.
func (b *Bus) StartMetricsTicker(ctx context.Context, reg *metrics.Registry) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snap := reg.GetSystem()
				if snap.CPUPercent == 0 {
					continue
				}
				b.Publish(Event{
					Type:  EventSystemMetrics,
					Topic: TopicMetrics,
					Payload: SystemMetricsPayload{
						CPUPercent: snap.CPUPercent,
						MemPercent: snap.MemPercent,
						DiskUsed:   snap.DiskUsedMB,
						DiskTotal:  snap.DiskTotalMB,
					},
				})
			}
		}
	}()
}
