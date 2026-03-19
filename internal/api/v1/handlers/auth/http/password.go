package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type ChangePasswordInput struct {
	Body struct {
		CurrentPassword string `json:"current_password" minLength:"1"`
		NewPassword     string `json:"new_password" minLength:"8"`
	}
}
type ChangePasswordOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (h *Handler) ChangePassword(ctx context.Context, input *ChangePasswordInput) (*ChangePasswordOutput, error) {
	abUser, ok := middleware.UserFromHumaCtx(ctx).(*models.User)
	if !ok || abUser == nil {
		return nil, huma401("unauthorized")
	}

	err := h.auth.ChangePassword(abUser, input.Body.CurrentPassword, input.Body.NewPassword)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditPasswordChange,
		ResourceID: abUser.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("email=%s reason=%v", abUser.Email, err),
	})
	if err != nil {
		switch err.Error() {
		case "wrong_current_password":
			return nil, huma.NewError(http.StatusUnauthorized, "current password is incorrect")
		default:
			return nil, err
		}
	}

	out := &ChangePasswordOutput{}
	out.Body.Message = "password changed successfully"
	return out, nil
}
