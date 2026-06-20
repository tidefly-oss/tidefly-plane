package system

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	// No auth — Docker healthcheck, K8s probes, uptime monitors
	huma.Register(api, httpx.Op("system-health", "GET", httpx.V1+"/system/health", "Liveness check", "System"), h.health)
	huma.Register(api, httpx.Op("system-info", "GET", httpx.V1+"/system/info", "Runtime info", "System", mw...), h.info)
	huma.Register(api, httpx.Op("system-ports", "GET", httpx.V1+"/system/ports", "Used host ports", "System", mw...), h.usedPorts)
	huma.Register(api, httpx.Op("system-metrics", "GET", httpx.V1+"/system/metrics", "System metrics", "System", mw...), h.getMetrics)
	huma.Register(api, httpx.Op("system-version", "GET", httpx.V1+"/system/version", "Check for updates", "System", mw...), h.version)
	huma.Register(api, httpx.Op("system-update", "POST", httpx.V1+"/admin/system/update", "Update Tidefly Plane", "System", adminMw...), h.updateSelf)
}

func (h *Handler) RegisterSSERoutes(r chi.Router, sseAuth func(http.Handler) http.Handler) {
	r.With(sseAuth).Get(httpx.V1+"/system/caddy-logs", h.CaddyLogs)
}
