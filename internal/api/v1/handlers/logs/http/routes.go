package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	r chi.Router,
	mw huma.Middlewares,
	adminMw huma.Middlewares,
	sseAuth func(http.Handler) http.Handler,
) {
	huma.Register(api, shared.Op("logs-app", "GET", "/api/v1/logs/app", "App logs", "Logs", mw...), h.ListAppLogs)
	huma.Register(api, shared.Op("logs-audit", "GET", "/api/v1/logs/audit", "Audit logs", "Logs", adminMw...), h.ListAuditLogs)
	r.With(sseAuth).Get("/api/v1/logs/app/stream", h.StreamAppLogs)
}
