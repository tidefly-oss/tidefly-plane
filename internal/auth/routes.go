package auth

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	op := func(id, method, path, summary string, mws ...func(huma.Context, func(huma.Context))) huma.Operation {
		return huma.Operation{
			OperationID: id,
			Method:      method,
			Path:        path,
			Summary:     summary,
			Tags:        []string{"Auth"},
			Middlewares: mws,
		}
	}

	// ── Public ────────────────────────────────────────────────────────────────
	huma.Register(api, op("auth-register", "POST", httpx.V1+"/auth/register", "Register"), h.register)
	huma.Register(api, op("auth-login", "POST", httpx.V1+"/auth/login", "Login"), h.login)
	huma.Register(api, op("auth-refresh", "POST", httpx.V1+"/auth/refresh", "Refresh token"), h.refresh)
	huma.Register(api, op("auth-logout", "POST", httpx.V1+"/auth/logout", "Logout"), h.logout)

	// ── Protected ─────────────────────────────────────────────────────────────
	huma.Register(api, op("auth-me", "GET", httpx.V1+"/auth/me", "Current user", mw...), h.me)
	huma.Register(api, op("auth-change-password", "POST", httpx.V1+"/auth/change-password", "Change password", mw...), h.changePassword)
	huma.Register(api, op("auth-logout-all", "POST", httpx.V1+"/auth/logout-all", "Logout all devices", mw...), h.logoutAll)
}
