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
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerfilters "github.com/docker/docker/api/types/filters"
	dockerimage "github.com/docker/docker/api/types/image"
	dockernetwork "github.com/docker/docker/api/types/network"
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

	// ── Pull all new images in parallel ──────────────────────────────────
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
			h.publishProgress("pulling", "pulling "+image)
			rc, err := cli.ImagePull(ctx, image, dockerimage.PullOptions{})
			if err != nil {
				h.publishFailed(err.Error())
				return
			}
			defer func(rc io.ReadCloser) { _ = rc.Close() }(rc)
			dec := json.NewDecoder(rc)
			for dec.More() {
				var msg struct {
					Status string `json:"status"`
				}
				if err := dec.Decode(&msg); err != nil {
					continue
				}
				h.publishProgress("pulling", msg.Status)
			}
			h.publishProgress("pulled", image+" ready")
		}(c.Name, imageName)
	}
	wg.Wait()

	// ── Recreate containers in order (UI first, then Plane) ───────────────
	restartOrder := []string{componentUI, componentPlane}
	for _, name := range restartOrder {
		var comp *ComponentVersion
		for i := range components {
			if components[i].Name == name {
				comp = &components[i]
				break
			}
		}
		if comp == nil {
			continue
		}
		cn := containerName(name)
		if cn == "" {
			continue
		}
		img, ok := componentImages[name]
		if !ok {
			continue
		}
		imageName := fmt.Sprintf("%s:%s", img, comp.Latest)

		if err := h.recreateContainer(ctx, cli, cn, imageName); err != nil {
			h.publishFailed(fmt.Sprintf("failed to recreate %s: %s", cn, err.Error()))
			continue
		}
		h.publishProgress("restarted", cn+" updated and running")
	}

	h.bus.Publish(eventbus.Event{
		Type:    eventbus.EventDeployDone,
		Topic:   eventbus.TopicDeploy,
		Payload: eventbus.DeployDonePayload{DeployID: systemUpdateID},
	})
	return nil
}

// recreateContainer stops, removes, recreates and starts a container with a new image
// while preserving all existing config (env, volumes, ports, networks, labels).
func (h *Handler) recreateContainer(ctx context.Context, cli *dockerclient.Client, containerName, newImage string) error {
	h.publishProgress("restarting", "recreating "+containerName)

	// ── Inspect existing container ────────────────────────────────────────
	info, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", containerName, err)
	}

	// ── Stop ──────────────────────────────────────────────────────────────
	timeout := 10
	if err := cli.ContainerStop(ctx, containerName, dockercontainer.StopOptions{Timeout: &timeout}); err != nil {
		h.log.Warnw("self_update", "stop container failed", "container", containerName, "error", err)
	}

	// ── Remove ────────────────────────────────────────────────────────────
	if err := cli.ContainerRemove(ctx, containerName, dockercontainer.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove %s: %w", containerName, err)
	}

	// ── Build new config from old ─────────────────────────────────────────
	containerCfg := info.Config
	containerCfg.Image = newImage

	hostCfg := info.HostConfig

	// ── Rebuild network config ────────────────────────────────────────────
	networkCfg := &dockernetwork.NetworkingConfig{
		EndpointsConfig: make(map[string]*dockernetwork.EndpointSettings),
	}
	for netName, endpoint := range info.NetworkSettings.Networks {
		networkCfg.EndpointsConfig[netName] = &dockernetwork.EndpointSettings{
			Aliases: endpoint.Aliases,
		}
	}

	// ── Create ────────────────────────────────────────────────────────────
	created, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, containerName)
	if err != nil {
		return fmt.Errorf("create %s: %w", containerName, err)
	}

	// ── Connect to additional networks ────────────────────────────────────
	for netName, endpoint := range info.NetworkSettings.Networks {
		if netName == hostCfg.NetworkMode.NetworkName() {
			continue
		}
		if err := cli.NetworkConnect(ctx, endpoint.NetworkID, created.ID, &dockernetwork.EndpointSettings{
			Aliases: endpoint.Aliases,
		}); err != nil {
			h.log.Warnw("self_update", "network connect failed", "network", netName, "error", err)
		}
	}

	// ── Start ─────────────────────────────────────────────────────────────
	if err := cli.ContainerStart(ctx, created.ID, dockercontainer.StartOptions{}); err != nil {
		return fmt.Errorf("start %s: %w", containerName, err)
	}

	// ── Cleanup old image ─────────────────────────────────────────────────
	go func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = cli.ImagesPrune(cleanCtx, dockerfilters.NewArgs(dockerfilters.Arg("dangling", "true")))
	}()

	return nil
}

func (h *Handler) publishProgress(step, message string) {
	h.bus.Publish(eventbus.Event{
		Type:  eventbus.EventDeployProgress,
		Topic: eventbus.TopicDeploy,
		Payload: eventbus.DeployProgressPayload{
			DeployID: systemUpdateID,
			Step:     step,
			Message:  message,
		},
	})
}

func (h *Handler) publishFailed(errMsg string) {
	h.bus.Publish(eventbus.Event{
		Type:  eventbus.EventDeployFailed,
		Topic: eventbus.TopicDeploy,
		Payload: eventbus.DeployFailedPayload{
			DeployID: systemUpdateID,
			Error:    errMsg,
		},
	})
}
