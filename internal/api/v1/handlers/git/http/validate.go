package http

import (
	"context"

	"github.com/tidefly-oss/tidefly-backend/internal/services/git/types"
)

type ValidateInput struct {
	ID string `path:"id"`
}
type ValidateOutput struct {
	Body struct {
		Valid bool   `json:"valid"`
		Error string `json:"error,omitempty"`
	}
}

func (h *Handler) Validate(ctx context.Context, input *ValidateInput) (*ValidateOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.RequireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	out := &ValidateOutput{}
	if verr := h.svc.ValidateIntegration(
		ctx,
		types.ProviderType(m.Provider),
		m.SecretEncrypted,
		m.BaseURL,
	); verr != nil {
		out.Body.Valid = false
		out.Body.Error = verr.Error()
	} else {
		out.Body.Valid = true
	}
	return out, nil
}
