package http

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/logger"
)

type ResetUserPasswordInput struct {
	ID string `path:"id" doc:"User ID"`
}
type ResetUserPasswordOutput struct {
	Body struct {
		TempPassword string `json:"temp_password"`
	}
}

func (h *Handler) ResetUserPassword(ctx context.Context, input *ResetUserPasswordInput) (*ResetUserPasswordOutput, error) {
	u, plain, err := h.users.ResetPassword(input.ID)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditAdminUserPasswordReset,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("email=%s", u.Email),
	})
	if err != nil {
		return nil, huma404("user not found")
	}

	out := &ResetUserPasswordOutput{}
	out.Body.TempPassword = plain
	return out, nil
}
