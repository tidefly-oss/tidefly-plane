package podman

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

type podmanEvent struct {
	Action string `json:"Action"`
	Type   string `json:"Type"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	Time   int64  `json:"time"`
	Status string `json:"status"`
}

var podmanRelevantActions = map[string]runtime.ContainerEventType{
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

func (p *Runtime) EventStream(ctx context.Context) (<-chan runtime.ContainerEvent, <-chan error) {
	out := make(chan runtime.ContainerEvent, 32)
	errc := make(chan error, 1)

	go func() {
		defer close(out)

		q := url.Values{}
		q.Set("stream", "true")
		q.Set("filters", filterQuery(map[string][]string{"type": {"container"}}))

		resp, err := p.c.get(ctx, "/libpod/events", q)
		if err != nil {
			errc <- err
			return
		}
		defer func() { _ = resp.Body.Close() }()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var evt podmanEvent
			if err := json.Unmarshal(line, &evt); err != nil {
				continue
			}

			if evt.Type != "container" {
				continue
			}

			evtType, ok := podmanRelevantActions[evt.Action]
			if !ok {
				continue
			}

			if evt.Actor.Attributes[runtime.LabelInternal] == runtime.LabelTrue {
				continue
			}

			id := evt.Actor.ID
			if len(id) > 12 {
				id = id[:12]
			}

			out <- runtime.ContainerEvent{
				Type:        evtType,
				ContainerID: id,
				Name:        evt.Actor.Attributes["name"],
				Image:       evt.Actor.Attributes["image"],
				Status:      runtime.EventToStatus(evtType),
				Time:        time.Unix(evt.Time, 0),
			}
		}

		if err := scanner.Err(); err != nil {
			if ctx.Err() == nil {
				errc <- fmt.Errorf("podman event stream: %w", err)
			}
		}
	}()

	return out, errc
}
