package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

const (
	systemUpdateID  = "system-update"
	caddyAdminURL   = "http://tidefly_caddy:2019"
	caddyHealthWait = 30 * time.Second
	containerHealth = 60 * time.Second
)

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
	componentCaddy: "tidefly/tidefly-caddy",
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
	case componentCaddy:
		return "tidefly_caddy"
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
			h.publishFailed(err.Error())
		}
	}()

	out := &UpdateOutput{}
	out.Body.Message = "update initiated — services will restart shortly"
	out.Body.Components = names
	return out, nil
}

func (h *Handler) pullAndRestartAll(ctx context.Context, components []ComponentVersion) error {
	h.log.Info("self_update", fmt.Sprintf("pullAndRestartAll started for %d components", len(components)))

	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		h.log.Error("self_update", "docker client init failed", err)
		return fmt.Errorf("docker client: %w", err)
	}
	h.log.Info("self_update", "docker client initialized")
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

	// ── Update Caddy first (config backup/restore) ────────────────────────
	for _, c := range components {
		if c.Name != componentCaddy {
			continue
		}
		imageName := fmt.Sprintf("%s:%s", componentImages[componentCaddy], c.Latest)
		if err := h.updateCaddy(ctx, cli, imageName); err != nil {
			h.publishFailed(fmt.Sprintf("failed to update caddy: %s", err.Error()))
			return err
		}
		h.publishProgress("restarted", "tidefly_caddy updated and running")
	}

	// ── Blue-Green for UI then Plane ──────────────────────────────────────
	blueGreenOrder := []string{componentUI, componentPlane}
	for _, name := range blueGreenOrder {
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

		isSelf := name == componentPlane
		if err := h.blueGreenUpdate(ctx, cli, cn, imageName, isSelf); err != nil {
			h.publishFailed(fmt.Sprintf("failed to update %s: %s", cn, err.Error()))
			continue
		}
		h.publishProgress("restarted", cn+" updated with zero downtime")
	}

	// For non-self-updates publish done immediately.
	// For self (plane), the new container publishes done after it starts.
	h.bus.Publish(eventbus.Event{
		Type:    eventbus.EventDeployDone,
		Topic:   eventbus.TopicDeploy,
		Payload: eventbus.DeployDonePayload{DeployID: systemUpdateID},
	})
	return nil
}

// blueGreenUpdate performs a zero-downtime update.
//
// isSelf=true means we are updating the Plane itself — in this case we must
// NOT stop/remove the old container from within the old process (it would kill
// us mid-execution). Instead we start the green container and rely on it to
// clean up the old one via the TIDEFLY_OLD_CONTAINER env var on first boot,
// OR we simply rename green → original and let Docker's restart policy handle
// the old one being orphaned (it has no restart policy after rename).
//
// Concretely for isSelf=true:
//  1. Start green with a unique temp name
//  2. Wait for it to be healthy
//  3. Rename green → original name  (Docker allows this even if original still exists
//     when we force-remove it first)
//  4. Force-remove the old container — at this point the old process dies, but
//     the new container is already running and serving traffic.
func (h *Handler) blueGreenUpdate(ctx context.Context, cli *dockerclient.Client, name, newImage string, isSelf bool) error {
	greenName := name + "_green"
	h.publishProgress("restarting", fmt.Sprintf("starting %s (blue-green)", greenName))

	// ── Inspect old container for config ─────────────────────────────────
	info, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", name, err)
	}

	// ── Remove stale green if exists ──────────────────────────────────────
	_ = cli.ContainerRemove(ctx, greenName, dockercontainer.RemoveOptions{Force: true})

	// ── Create green container with new image ─────────────────────────────
	containerCfg := info.Config
	containerCfg.Image = newImage
	hostCfg := info.HostConfig

	// Remove host port bindings for the green container — it runs alongside
	// the old container so host ports would conflict. Caddy reaches it via
	// container name on the Docker network, not via host ports.
	hostCfg.PortBindings = nil
	hostCfg.PublishAllPorts = false

	networkCfg := &dockernetwork.NetworkingConfig{
		EndpointsConfig: make(map[string]*dockernetwork.EndpointSettings),
	}
	for netName, endpoint := range info.NetworkSettings.Networks {
		networkCfg.EndpointsConfig[netName] = &dockernetwork.EndpointSettings{
			Aliases: endpoint.Aliases,
		}
	}

	created, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, greenName)
	if err != nil {
		return fmt.Errorf("create %s: %w", greenName, err)
	}

	// Connect additional networks
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

	// ── Start green ───────────────────────────────────────────────────────
	if err := cli.ContainerStart(ctx, created.ID, dockercontainer.StartOptions{}); err != nil {
		_ = cli.ContainerRemove(ctx, created.ID, dockercontainer.RemoveOptions{Force: true})
		return fmt.Errorf("start %s: %w", greenName, err)
	}

	// ── Wait for green to be healthy ──────────────────────────────────────
	h.publishProgress("restarting", fmt.Sprintf("waiting for %s to be healthy", greenName))
	if err := h.waitContainerHealthy(ctx, cli, created.ID, containerHealth); err != nil {
		_ = cli.ContainerStop(ctx, created.ID, dockercontainer.StopOptions{})
		_ = cli.ContainerRemove(ctx, created.ID, dockercontainer.RemoveOptions{Force: true})
		return fmt.Errorf("%s unhealthy, rolled back: %w", greenName, err)
	}

	// ── Self-update: force-remove old first, then rename green → original ─
	// For isSelf=true: we force-remove the old container BEFORE rename.
	// This kills our own process but the new container is already healthy
	// and serving traffic. Rename is best-effort — Docker may not complete
	// it but the green container is already running under greenName.
	if isSelf {
		h.publishProgress("restarting", fmt.Sprintf("replacing %s with new version (process will restart)", name))
		// Force-remove old — this kills the current process for componentPlane.
		// The green container keeps running. On next startup it will have the
		// original name because we rename it just before the remove.
		_ = cli.ContainerRename(ctx, created.ID, name) // best-effort: may fail if old still exists
		_ = cli.ContainerStop(ctx, name+"_old_placeholder", dockercontainer.StopOptions{})

		// Rename old → _old so we can rename green → original
		oldTmp := name + "_old"
		_ = cli.ContainerRename(ctx, name, oldTmp)

		// Rename green → original name
		if err := cli.ContainerRename(ctx, created.ID, name); err != nil {
			// Green is running under greenName — acceptable fallback
			h.log.Warnw("self_update", "rename green failed, running under greenName", "error", err)
		}

		// Now remove old — this kills us. No code runs after this in the old process.
		_ = cli.ContainerRemove(ctx, oldTmp, dockercontainer.RemoveOptions{Force: true})
		return nil
	}

	// ── Normal (non-self) update: stop old, rename green → original ───────
	h.publishProgress("restarting", fmt.Sprintf("stopping old %s", name))
	_ = cli.ContainerStop(ctx, name, dockercontainer.StopOptions{Timeout: new(10)})
	if err := cli.ContainerRemove(ctx, name, dockercontainer.RemoveOptions{Force: true}); err != nil {
		h.log.Warnw("self_update", "remove old container failed", "container", name, "error", err)
	}

	if err := cli.ContainerRename(ctx, created.ID, name); err != nil {
		return fmt.Errorf("rename %s → %s: %w", greenName, name, err)
	}

	// ── Cleanup dangling images ───────────────────────────────────────────
	go func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = cli.ImagesPrune(cleanCtx, dockerfilters.NewArgs(dockerfilters.Arg("dangling", "true")))
	}()

	return nil
}

func (h *Handler) waitContainerHealthy(ctx context.Context, cli *dockerclient.Client, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		inspect, err := cli.ContainerInspect(ctx, id)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if inspect.State.Health == nil {
			if inspect.State.Running {
				return nil
			}
		} else {
			switch inspect.State.Health.Status {
			case "healthy":
				return nil
			case "unhealthy":
				return fmt.Errorf("container reported unhealthy")
			}
		}
		if !inspect.State.Running {
			return fmt.Errorf("container exited during startup")
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("container not healthy after %s", timeout)
}

func (h *Handler) updateCaddy(ctx context.Context, cli *dockerclient.Client, newImage string) error {
	h.publishProgress("restarting", "backing up Caddy config")
	cfg, err := h.backupCaddyConfig(ctx)
	if err != nil {
		h.log.Warnw("self_update", "caddy config backup failed", "error", err)
	}

	h.publishProgress("restarting", "recreating tidefly_caddy")
	if err := h.recreateContainer(ctx, cli, "tidefly_caddy", newImage); err != nil {
		return fmt.Errorf("recreate caddy: %w", err)
	}

	h.publishProgress("restarting", "waiting for Caddy to be ready")
	if err := h.waitCaddyHealthy(ctx, caddyHealthWait); err != nil {
		return fmt.Errorf("caddy health wait: %w", err)
	}

	if cfg != nil {
		h.publishProgress("restarting", "restoring Caddy config")
		if err := h.restoreCaddyConfig(ctx, cfg); err != nil {
			h.log.Warnw("self_update", "caddy config restore failed", "error", err)
		}
	}
	return nil
}

func (h *Handler) backupCaddyConfig(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, caddyAdminURL+"/config/", nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("caddy config backup: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (h *Handler) restoreCaddyConfig(ctx context.Context, cfg []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, caddyAdminURL+"/load", bytes.NewReader(cfg))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("caddy config restore: status %d", resp.StatusCode)
	}
	return nil
}

func (h *Handler) waitCaddyHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, caddyAdminURL+"/", nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("caddy not ready after %s", timeout)
}

func (h *Handler) recreateContainer(ctx context.Context, cli *dockerclient.Client, containerName, newImage string) error {
	info, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", containerName, err)
	}

	_ = cli.ContainerStop(ctx, containerName, dockercontainer.StopOptions{Timeout: new(10)})

	if err := cli.ContainerRemove(ctx, containerName, dockercontainer.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove %s: %w", containerName, err)
	}

	containerCfg := info.Config
	containerCfg.Image = newImage
	hostCfg := info.HostConfig

	networkCfg := &dockernetwork.NetworkingConfig{
		EndpointsConfig: make(map[string]*dockernetwork.EndpointSettings),
	}
	for netName, endpoint := range info.NetworkSettings.Networks {
		networkCfg.EndpointsConfig[netName] = &dockernetwork.EndpointSettings{
			Aliases: endpoint.Aliases,
		}
	}

	created, err := cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, containerName)
	if err != nil {
		return fmt.Errorf("create %s: %w", containerName, err)
	}

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

	if err := cli.ContainerStart(ctx, created.ID, dockercontainer.StartOptions{}); err != nil {
		return fmt.Errorf("start %s: %w", containerName, err)
	}

	go func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = cli.ImagesPrune(cleanCtx, dockerfilters.NewArgs(dockerfilters.Arg("dangling", "true")))
	}()

	return nil
}

func (h *Handler) publishProgress(step, message string) {
	h.log.Info("self_update", fmt.Sprintf("[%s] %s", step, message))
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
