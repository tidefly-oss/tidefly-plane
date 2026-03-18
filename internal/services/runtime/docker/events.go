package docker

import (
	"context"
	"time"

	dockerevents "github.com/docker/docker/api/types/events"
	dockerfilters "github.com/docker/docker/api/types/filters"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

var relevantActions = map[dockerevents.Action]runtime.ContainerEventType{
	"start":   runtime.EventStart,
	"stop":    runtime.EventStop,
	"die":     runtime.EventDie,
	"kill":    runtime.EventKill,
	"restart": runtime.EventRestart,
	"pause":   runtime.EventPause,
	"unpause": runtime.EventUnpause,
	"destroy": runtime.EventDestroy,
	"create":  runtime.EventCreate,
	"oom":     runtime.EventOOM,
}

func (d *Runtime) EventStream(ctx context.Context) (<-chan runtime.ContainerEvent, <-chan error) {
	out := make(chan runtime.ContainerEvent, 32)
	errc := make(chan error, 1)

	f := dockerfilters.NewArgs()
	f.Add("type", "container")

	msgCh, errCh := d.client.Events(ctx, dockerevents.ListOptions{Filters: f})

	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return

			case err := <-errCh:
				if err != nil {
					errc <- err
				}
				return

			case msg := <-msgCh:
				evtType, ok := relevantActions[msg.Action]
				if !ok {
					continue
				}
				if msg.Actor.Attributes["tidefly.internal"] == "true" {
					continue
				}

				id := msg.Actor.ID
				if len(id) > 12 {
					id = id[:12]
				}

				out <- runtime.ContainerEvent{
					Type:        evtType,
					ContainerID: id,
					Name:        msg.Actor.Attributes["name"],
					Image:       msg.Actor.Attributes["image"],
					Status:      runtime.EventToStatus(evtType),
					Time:        time.Unix(msg.Time, 0),
				}
			}
		}
	}()

	return out, errc
}
