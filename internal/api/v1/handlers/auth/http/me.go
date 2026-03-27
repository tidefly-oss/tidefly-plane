package http

import (
	"context"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/auth/mapper"
)

type CurrentUserInput struct{}
type CurrentUserOutput struct {
	Body struct {
		User mapper.CurrentUserResponse `json:"user"`
	}
}

func (h *Handler) CurrentUser(ctx context.Context, _ *CurrentUserInput) (*CurrentUserOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma401("unauthorized")
	}

	u, err := h.auth.GetFreshUser(claims.UserID)
	if err != nil {
		return nil, huma401("user not found")
	}

	out := &CurrentUserOutput{}
	out.Body.User = mapper.ToCurrentUserResponse(&u)
	return out, nil
}
