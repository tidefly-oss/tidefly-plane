package http

import (
	"context"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/auth/mapper"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type CurrentUserInput struct{}
type CurrentUserOutput struct {
	Body struct {
		User mapper.CurrentUserResponse `json:"user"`
	}
}

func (h *Handler) CurrentUser(ctx context.Context, _ *CurrentUserInput) (*CurrentUserOutput, error) {
	abUser, ok := middleware.UserFromHumaCtx(ctx).(*models.User)
	if !ok || abUser == nil {
		return nil, huma401("unauthorized")
	}
	u, err := h.auth.GetFreshUser(abUser.ID)
	if err != nil {
		return nil, huma401("user not found")
	}
	out := &CurrentUserOutput{}
	out.Body.User = mapper.ToCurrentUserResponse(&u)
	return out, nil
}
