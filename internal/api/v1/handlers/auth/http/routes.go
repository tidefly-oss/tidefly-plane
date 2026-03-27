package http

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	// ── Public ────────────────────────────────────────────────────────────────
	huma.Register(api, shared.Op("auth-register", "POST", "/api/v1/auth/register", "Register"), h.Register)
	huma.Register(api, shared.Op("auth-login", "POST", "/api/v1/auth/login", "Login"), h.Login)
	huma.Register(api, shared.Op("auth-refresh", "POST", "/api/v1/auth/refresh", "Refresh token"), h.Refresh)
	huma.Register(api, shared.Op("auth-logout", "POST", "/api/v1/auth/logout", "Logout"), h.Logout)

	// ── Protected ─────────────────────────────────────────────────────────────
	huma.Register(api, shared.Op("auth-me", "GET", "/api/v1/auth/me", "Current user", mw...), h.CurrentUser)
	huma.Register(
		api,
		shared.Op("auth-change-password", "POST", "/api/v1/auth/change-password", "Change password", mw...),
		h.ChangePassword,
	)
	huma.Register(
		api,
		shared.Op("auth-logout-all", "POST", "/api/v1/auth/logout-all", "Logout all devices", mw...),
		h.LogoutAll,
	)
}
