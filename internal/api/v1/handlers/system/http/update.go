package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	dockertypes "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
)

const systemUpdateID = "system-update"

type UpdateInput struct{}

type UpdateOutput struct {
	Body struct {
		Message    string   `json:"message"`
		Components []string `json:"components"`
	}
}

var componentImages = map[string]string{
	componentPlane: "tidefly/tidefly-plane",
	componentUI:    "tidefly/tidefly-ui",
}

func containerName(component string) string {
	name := os.Getenv("TIDEFLY_CONTAINER_NAME")
	switch component {
	case componentPlane:
		if name != "" {
			return name
		}
		return "tidefly_backend"
	case componentUI:
		return "tidefly_ui"
	}
	return ""
}

func (h *Handler) UpdateSelf(ctx context.Context, _ *UpdateInput) (*UpdateOutput, error) {
	info, err := h.fetchVersionInfo(ctx)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("cannot reach GitHub: " + err.Error())
	}
	if !info.AnyUpdateAvailable {
		out := &UpdateOutput{}
		out.Body.Message = "all components are up to date"
		return out, nil
	}

	var toUpdate []ComponentVersion
	for _, c := range info.Components {
		if c.UpdateAvailable {
			toUpdate = append(toUpdate, c)
		}
	}

	var names []string
	for _, c := range toUpdate {
		names = append(names, c.Name)
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := h.pullAndRestartAll(bgCtx, toUpdate); err != nil {
			h.log.Errorw("self_update", "update failed", "error", err.Error())
			h.bus.Publish(eventbus.Event{
				Type:  eventbus.EventDeployFailed,
				Topic: eventbus.TopicDeploy,
				Payload: eventbus.DeployFailedPayload{
					DeployID: systemUpdateID,
					Error:    err.Error(),
				},
			})
		}
	}()

	out := &UpdateOutput{}
	out.Body.Message = "update initiated — services will restart shortly"
	out.Body.Components = names
	return out, nil
}

func (h *Handler) pullAndRestartAll(ctx context.Context, components []ComponentVersion) error {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer func(cli *dockerclient.Client) { _ = cli.Close() }(cli)

	var wg sync.WaitGroup
	for _, c := range components {
		img, ok := componentImages[c.Name]
		if !ok {
			continue
		}
		imageName := fmt.Sprintf("%s:%s", img, c.Latest)
		wg.Add(1)
		go func(name, image string) {
			defer wg.Done()
			h.bus.Publish(eventbus.Event{
				Type:  eventbus.EventDeployProgress,
				Topic: eventbus.TopicDeploy,
				Payload: eventbus.DeployProgressPayload{
					DeployID: systemUpdateID,
					Step:     "pulling",
					Message:  "pulling " + image,
				},
			})
			rc, err := cli.ImagePull(ctx, image, dockerimage.PullOptions{})
			if err != nil {
				h.bus.Publish(eventbus.Event{
					Type:  eventbus.EventDeployFailed,
					Topic: eventbus.TopicDeploy,
					Payload: eventbus.DeployFailedPayload{
						DeployID: systemUpdateID,
						Error:    err.Error(),
					},
				})
				return
			}
			defer func(rc io.ReadCloser) { _ = rc.Close() }(rc)
			dec := json.NewDecoder(rc)
			for dec.More() {
				var msg struct {
					Status string `json:"status"`
					ID     string `json:"id"`
				}
				if err := dec.Decode(&msg); err != nil {
					continue
				}
				h.bus.Publish(eventbus.Event{
					Type:  eventbus.EventDeployProgress,
					Topic: eventbus.TopicDeploy,
					Payload: eventbus.DeployProgressPayload{
						DeployID: systemUpdateID,
						Step:     "pulling",
						Message:  msg.Status,
					},
				})
			}
			h.bus.Publish(eventbus.Event{
				Type:  eventbus.EventDeployProgress,
				Topic: eventbus.TopicDeploy,
				Payload: eventbus.DeployProgressPayload{
					DeployID: systemUpdateID,
					Step:     "pulled",
					Message:  image + " ready",
				},
			})
		}(c.Name, imageName)
	}
	wg.Wait()

	restartOrder := []string{componentUI, componentPlane}
	for _, name := range restartOrder {
		cn := containerName(name)
		if cn == "" {
			continue
		}
		if _, err := cli.ContainerInspect(ctx, cn); err != nil {
			continue
		}
		h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventDeployProgress,
			Topic: eventbus.TopicDeploy,
			Payload: eventbus.DeployProgressPayload{
				DeployID: systemUpdateID,
				Step:     "restarting",
				Message:  "restarting " + cn,
			},
		})
		if err := cli.ContainerRestart(ctx, cn, dockertypes.StopOptions{Timeout: new(5)}); err != nil {
			h.bus.Publish(eventbus.Event{
				Type:  eventbus.EventDeployFailed,
				Topic: eventbus.TopicDeploy,
				Payload: eventbus.DeployFailedPayload{
					DeployID: "system-update",
					Error:    err.Error(),
				},
			})
			continue
		}
		h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventDeployProgress,
			Topic: eventbus.TopicDeploy,
			Payload: eventbus.DeployProgressPayload{
				DeployID: systemUpdateID,
				Step:     "restarted",
				Message:  cn + " restarted",
			},
		})
	}

	h.bus.Publish(eventbus.Event{
		Type:  eventbus.EventDeployDone,
		Topic: eventbus.TopicDeploy,
		Payload: eventbus.DeployDonePayload{
			DeployID: systemUpdateID,
		},
	})
	return nil
}
