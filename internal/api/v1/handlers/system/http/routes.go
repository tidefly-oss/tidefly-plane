package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	// No auth — Docker healthcheck, K8s probes, uptime monitors
	huma.Register(
		api,
		shared.Op("system-health", "GET", "/api/v1/system/health", "Liveness check", "System"),
		h.Health,
	)
	huma.Register(
		api,
		shared.Op("system-info", "GET", "/api/v1/system/info", "Runtime info", "System", mw...),
		h.Info,
	)
	huma.Register(
		api,
		shared.Op("system-ports", "GET", "/api/v1/system/ports", "Used host ports", "System", mw...),
		h.UsedPorts,
	)
	huma.Register(
		api,
		shared.Op("system-version", "GET", "/api/v1/system/version", "Check for updates", "System", mw...),
		h.Version,
	)
	huma.Register(
		api,
		shared.Op("system-update", "POST", "/api/v1/admin/system/update", "Update Tidefly Plane", "System", adminMw...),
		h.UpdateSelf,
	)
}

func (h *Handler) RegisterSSERoutes(e *echo.Echo, echoSSE echo.MiddlewareFunc, echoInject echo.MiddlewareFunc) {
	// Caddy logs stay SSE — unidirectional log stream
	e.GET("/api/v1/system/caddy-logs", func(c *echo.Context) error {
		return h.CaddyLogs(c)
	}, echoSSE, echoInject)
}
