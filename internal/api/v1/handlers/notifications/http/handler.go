package http

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
)

type Handler struct {
	svc *notifications.Service
}

func New(svc *notifications.Service) *Handler {
	return &Handler{svc: svc}
}

type ListInput struct{}
type ListOutput struct {
	Body []models.Notification
}

type ListAllInput struct{}
type ListAllOutput struct {
	Body []models.Notification
}

type CountInput struct{}
type CountOutput struct {
	Body struct {
		Count int64 `json:"count"`
	}
}

type AcknowledgeInput struct {
	ID string `path:"id"`
}

type DeleteInput struct {
	ID string `path:"id"`
}

type DeleteAcknowledgedInput struct{}

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

func (h *Handler) Count(ctx context.Context, _ *CountInput) (*CountOutput, error) {
	count, err := h.svc.UnreadCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("count notifications: %w", err)
	}
	out := &CountOutput{}
	out.Body.Count = count
	return out, nil
}

func (h *Handler) Acknowledge(ctx context.Context, input *AcknowledgeInput) (*struct{}, error) {
	if err := h.svc.Acknowledge(ctx, input.ID); err != nil {
		return nil, fmt.Errorf("acknowledge notification: %w", err)
	}
	return nil, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	if err := h.svc.Delete(ctx, input.ID); err != nil {
		return nil, fmt.Errorf("delete notification: %w", err)
	}
	return nil, nil
}

func (h *Handler) DeleteAcknowledged(ctx context.Context, _ *DeleteAcknowledgedInput) (*struct{}, error) {
	if err := h.svc.DeleteAll(ctx); err != nil {
		return nil, fmt.Errorf("delete acknowledged notifications: %w", err)
	}
	return nil, nil
}
