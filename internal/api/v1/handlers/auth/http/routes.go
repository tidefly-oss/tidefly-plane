package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-backend/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, shared.Op("auth-me", "GET", "/api/v1/auth/me", "Current user", mw...), h.CurrentUser)
	huma.Register(
		api,
		shared.Op("auth-change-password", "POST", "/api/v1/auth/change-password", "Change password", mw...),
		h.ChangePassword,
	)
}
