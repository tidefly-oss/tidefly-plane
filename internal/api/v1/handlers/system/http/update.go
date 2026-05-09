package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	dockertypes "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/labstack/echo/v5"
)

// ── Progress Bus ──────────────────────────────────────────────────────────────

type progressEvent struct {
	Status    string `json:"status"` // "pulling" | "done" | "error"
	Message   string `json:"message"`
	Layer     string `json:"layer,omitempty"`
	Component string `json:"component,omitempty"` // "plane" | "ui" | "caddy"
}

type progressBus struct {
	mu   sync.RWMutex
	subs map[chan progressEvent]struct{}
}

var updateBus = &progressBus{
	subs: make(map[chan progressEvent]struct{}),
}

func (b *progressBus) subscribe() chan progressEvent {
	ch := make(chan progressEvent, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *progressBus) unsubscribe(ch chan progressEvent) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

func (b *progressBus) publish(ev progressEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// ── Version Check ─────────────────────────────────────────────────────────────

type VersionInput struct{}

type VersionOutput struct {
	Body versionInfo
}

func (h *Handler) Version(ctx context.Context, _ *VersionInput) (*VersionOutput, error) {
	info, err := h.fetchVersionInfo(ctx)
	if err != nil {
		h.log.Warnw("version_check", "failed to fetch version info", "error", err.Error())
		return &VersionOutput{Body: versionInfo{
			Components: []ComponentVersion{{
				Name:    "plane",
				Current: currentVersion(),
				Latest:  "unknown",
			}},
			AnyUpdateAvailable: false,
		}}, nil
	}
	return &VersionOutput{Body: *info}, nil
}

// ── Self-Update ───────────────────────────────────────────────────────────────

type UpdateInput struct{}

type UpdateOutput struct {
	Body struct {
		Message    string   `json:"message"`
		Components []string `json:"components"`
	}
}

// component → image map
var componentImages = map[string]string{
	"plane": "tidefly/tidefly-plane",
	"ui":    "tidefly/tidefly-ui",
}

// component → container name map (dev + prod)
func containerName(component string) string {
	name := os.Getenv("TIDEFLY_CONTAINER_NAME")
	switch component {
	case "plane":
		if name != "" {
			return name
		}
		return "tidefly_backend"
	case "ui":
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
			updateBus.publish(progressEvent{Status: "error", Message: err.Error()})
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
	defer func(cli *dockerclient.Client) {
		_ = cli.Close()
	}(cli)

	// Pull all images first (parallel)
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
			updateBus.publish(progressEvent{Status: "pulling", Message: "pulling " + image, Component: name})
			rc, err := cli.ImagePull(ctx, image, dockerimage.PullOptions{})
			if err != nil {
				updateBus.publish(progressEvent{Status: "error", Message: err.Error(), Component: name})
				return
			}
			defer func(rc io.ReadCloser) {
				_ = rc.Close()
			}(rc)
			dec := json.NewDecoder(rc)
			for dec.More() {
				var msg struct {
					Status string `json:"status"`
					ID     string `json:"id"`
				}
				if err := dec.Decode(&msg); err != nil {
					continue
				}
				updateBus.publish(progressEvent{Status: "pulling", Message: msg.Status, Layer: msg.ID, Component: name})
			}
			updateBus.publish(progressEvent{Status: "pulled", Message: image + " ready", Component: name})
		}(c.Name, imageName)
	}
	wg.Wait()

	// Restart in order: ui first, plane last
	restartOrder := []string{"ui", "plane"}
	for _, name := range restartOrder {
		cn := containerName(name)
		if cn == "" {
			continue
		}
		// Check if container exists
		_, err := cli.ContainerInspect(ctx, cn)
		if err != nil {
			continue // not running, skip
		}
		updateBus.publish(progressEvent{Status: "restarting", Message: "restarting " + cn, Component: name})
		if err := cli.ContainerRestart(ctx, cn, dockertypes.StopOptions{Timeout: new(5)}); err != nil {
			updateBus.publish(progressEvent{Status: "error", Message: err.Error(), Component: name})
			continue
		}
		updateBus.publish(progressEvent{Status: "restarted", Message: cn + " restarted", Component: name})
	}

	updateBus.publish(progressEvent{Status: "done", Message: "all updates applied"})
	return nil
}

// ── SSE Update Progress ───────────────────────────────────────────────────────

func (h *Handler) UpdateProgress(c *echo.Context) error {
	ctx := (*c).Request().Context()
	resp := (*c).Response()

	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)

	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ch := updateBus.subscribe()
	defer updateBus.unsubscribe(ch)

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			data, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(resp, "data: %s\n\n", data)
			flusher.Flush()
			if ev.Status == "done" || ev.Status == "error" {
				return nil
			}
		case <-ticker.C:
			_, _ = fmt.Fprintf(resp, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}
