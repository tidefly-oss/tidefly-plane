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
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
	"github.com/labstack/echo/v5"
)

// ── Progress Bus ──────────────────────────────────────────────────────────────

type progressEvent struct {
	Status  string `json:"status"` // "pulling" | "done" | "error"
	Message string `json:"message"`
	Layer   string `json:"layer,omitempty"` // Docker layer id
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
	info, err := fetchLatestVersion(ctx)
	if err != nil {
		h.log.Warnw("version_check", "failed to fetch latest version", "error", err.Error())
		return &VersionOutput{Body: versionInfo{
			Current:         currentVersion(),
			Latest:          "unknown",
			UpdateAvailable: false,
		}}, nil
	}
	return &VersionOutput{Body: *info}, nil
}

// ── Self-Update ───────────────────────────────────────────────────────────────

type UpdateInput struct{}

type UpdateOutput struct {
	Body struct {
		Message string `json:"message"`
		Version string `json:"version"`
	}
}

func (h *Handler) UpdateSelf(ctx context.Context, _ *UpdateInput) (*UpdateOutput, error) {
	info, err := fetchLatestVersion(ctx)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("cannot reach GitHub to check for updates: " + err.Error())
	}
	if !info.UpdateAvailable {
		out := &UpdateOutput{}
		out.Body.Message = "already on latest version"
		out.Body.Version = info.Current
		return out, nil
	}

	containerName := os.Getenv("TIDEFLY_CONTAINER_NAME")
	if containerName == "" {
		containerName = "tidefly_backend"
	}

	imageName := fmt.Sprintf("tidefly/tidefly-plane:%s", info.Latest)

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := pullAndRestart(bgCtx, containerName, imageName, h); err != nil {
			h.log.Errorw("self_update", "update failed", "error", err.Error())
			updateBus.publish(progressEvent{Status: "error", Message: err.Error()})
		}
	}()

	out := &UpdateOutput{}
	out.Body.Message = fmt.Sprintf("update to %s initiated — plane will restart shortly", info.Latest)
	out.Body.Version = info.Latest
	return out, nil
}

func pullAndRestart(ctx context.Context, containerName, imageName string, h *Handler) error {
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

	h.log.Info("self_update", "pulling image", "image", imageName)
	updateBus.publish(progressEvent{Status: "pulling", Message: "pulling " + imageName})

	rc, err := cli.ImagePull(ctx, imageName, dockerimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("image pull: %w", err)
	}
	defer func(rc io.ReadCloser) {
		_ = rc.Close()
	}(rc)

	// Stream Docker pull progress to SSE bus
	dec := json.NewDecoder(rc)
	for dec.More() {
		var msg struct {
			Status string `json:"status"`
			ID     string `json:"id"`
		}
		if err := dec.Decode(&msg); err != nil {
			continue
		}
		updateBus.publish(progressEvent{
			Status:  "pulling",
			Message: msg.Status,
			Layer:   msg.ID,
		})
	}

	h.log.Info("self_update", "image pulled, restarting", "container", containerName)
	updateBus.publish(progressEvent{Status: "pulling", Message: "restarting container..."})

	if err := cli.ContainerRestart(ctx, containerName, dockercontainer.StopOptions{Timeout: new(5)}); err != nil {
		return fmt.Errorf("container restart: %w", err)
	}

	updateBus.publish(progressEvent{Status: "done", Message: "restart triggered"})
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
