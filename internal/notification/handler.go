package notification

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type listOutput struct {
	Body []models.Notification
}

func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	ns, err := h.svc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	if ns == nil {
		ns = []models.Notification{}
	}
	return &listOutput{Body: ns}, nil
}

type listAllOutput struct {
	Body []models.Notification
}

func (h *Handler) listAll(ctx context.Context, _ *struct{}) (*listAllOutput, error) {
	ns, err := h.svc.ListAll(ctx, 200)
	if err != nil {
		return nil, fmt.Errorf("list all notifications: %w", err)
	}
	if ns == nil {
		ns = []models.Notification{}
	}
	return &listAllOutput{Body: ns}, nil
}

type countOutput struct {
	Body struct {
		Count int64 `json:"count"`
	}
}

func (h *Handler) count(ctx context.Context, _ *struct{}) (*countOutput, error) {
	count, err := h.svc.UnreadCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("count notifications: %w", err)
	}
	out := &countOutput{}
	out.Body.Count = count
	return out, nil
}

type acknowledgeInput struct {
	ID string `path:"id"`
}

func (h *Handler) acknowledge(ctx context.Context, input *acknowledgeInput) (*struct{}, error) {
	if err := h.svc.Acknowledge(ctx, input.ID); err != nil {
		return nil, fmt.Errorf("acknowledge notification: %w", err)
	}
	return nil, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	if err := h.svc.Delete(ctx, input.ID); err != nil {
		return nil, fmt.Errorf("delete notification: %w", err)
	}
	return nil, nil
}

func (h *Handler) deleteAcknowledged(ctx context.Context, _ *struct{}) (*struct{}, error) {
	if err := h.svc.DeleteAll(ctx); err != nil {
		return nil, fmt.Errorf("delete acknowledged notifications: %w", err)
	}
	return nil, nil
}
