package log

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	r chi.Router,
	mw huma.Middlewares,
	adminMw huma.Middlewares,
	sseAuth func(http.Handler) http.Handler,
) {
	huma.Register(api, httpx.Op("logs-app", "GET", httpx.V1+"/logs/app", "App logs", "Logs", mw...), h.listAppLogs)
	huma.Register(api, httpx.Op("logs-audit", "GET", httpx.V1+"/logs/audit", "Audit logs", "Logs", adminMw...), h.listAuditLogs)
	r.With(sseAuth).Get(httpx.V1+"/logs/app/stream", h.streamAppLogs)
}
