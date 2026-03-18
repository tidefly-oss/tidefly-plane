package notifications

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
)

type Handler struct {
	svc *notifications.Service
}

func New(svc *notifications.Service) *Handler {
	return &Handler{svc: svc}
}

// ── List ──────────────────────────────────────────────────────────────────────

type ListInput struct{}
type ListOutput struct {
	Body []models.Notification
}

func (h *Handler) List(ctx context.Context, _ *ListInput) (*ListOutput, error) {
	ns, err := h.svc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	if ns == nil {
		ns = []models.Notification{}
	}
	return &ListOutput{Body: ns}, nil
}

// ── ListAll ───────────────────────────────────────────────────────────────────

type ListAllInput struct{}
type ListAllOutput struct {
	Body []models.Notification
}

func (h *Handler) ListAll(ctx context.Context, _ *ListAllInput) (*ListAllOutput, error) {
	ns, err := h.svc.ListAll(ctx, 200)
	if err != nil {
		return nil, fmt.Errorf("list all notifications: %w", err)
	}
	if ns == nil {
		ns = []models.Notification{}
	}
	return &ListAllOutput{Body: ns}, nil
}

// ── Count ─────────────────────────────────────────────────────────────────────

type CountInput struct{}
type CountOutput struct {
	Body struct {
		Count int64 `json:"count"`
	}
}

func (h *Handler) Count(ctx context.Context, _ *CountInput) (*CountOutput, error) {
	count, err := h.svc.UnreadCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("count notifications: %w", err)
	}
	out := &CountOutput{}
	out.Body.Count = count
	return out, nil
}

// ── Acknowledge ───────────────────────────────────────────────────────────────

type AcknowledgeInput struct {
	ID string `path:"id" doc:"Notification ID"`
}

func (h *Handler) Acknowledge(ctx context.Context, input *AcknowledgeInput) (*struct{}, error) {
	if err := h.svc.Acknowledge(ctx, input.ID); err != nil {
		return nil, fmt.Errorf("acknowledge notification: %w", err)
	}
	return nil, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type DeleteInput struct {
	ID string `path:"id" doc:"Notification ID"`
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	if err := h.svc.Delete(ctx, input.ID); err != nil {
		return nil, fmt.Errorf("delete notification: %w", err)
	}
	return nil, nil
}

// ── DeleteAcknowledged ────────────────────────────────────────────────────────

type DeleteAcknowledgedInput struct{}

func (h *Handler) DeleteAcknowledged(ctx context.Context, _ *DeleteAcknowledgedInput) (*struct{}, error) {
	if err := h.svc.DeleteAll(ctx); err != nil {
		return nil, fmt.Errorf("delete acknowledged notifications: %w", err)
	}
	return nil, nil
}

// ── Stream (bleibt Echo — SSE) ────────────────────────────────────────────────

func (h *Handler) Stream(c *echo.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-c.Request().Context().Done()
		cancel()
	}()

	resp := c.Response()
	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)

	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	sendEvent := func(event, data string) {
		fmt.Fprintf(resp, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	ch, unsub := h.svc.Subscribe(ctx)
	defer unsub()

	sendEvent("ping", "connected")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case data, ok := <-ch:
			if !ok {
				return nil
			}
			sendEvent("notification", string(data))
		case <-ticker.C:
			sendEvent("ping", "keepalive")
		}
	}
}
