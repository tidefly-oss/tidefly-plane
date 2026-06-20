package git

import (
	"context"
)

type validateInput struct {
	ID string `path:"id"`
}

type validateOutput struct {
	Body struct {
		Valid bool   `json:"valid"`
		Error string `json:"error,omitempty"`
	}
}

func (h *Handler) validate(ctx context.Context, input *validateInput) (*validateOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.store.RequireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	out := &validateOutput{}
	if verr := h.svc.ValidateIntegration(ctx, ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL); verr != nil {
		out.Body.Valid = false
		out.Body.Error = verr.Error()
	} else {
		out.Body.Valid = true
	}
	return out, nil
}
