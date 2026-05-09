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
		shared.Op("system-overview", "GET", "/api/v1/system/overview", "Dashboard overview", "System", mw...),
		h.Overview,
	)
	huma.Register(
		api,
		shared.Op("system-metrics", "GET", "/api/v1/system/metrics", "Current system metrics", "System", mw...),
		h.Metrics,
	)
	huma.Register(
		api,
		shared.Op("system-ports", "GET", "/api/v1/system/ports", "Used host ports", "System", mw...),
		h.UsedPorts,
	)

	// Version check — jeder eingeloggte User (für Update-Banner im Frontend)
	huma.Register(
		api,
		shared.Op("system-version", "GET", "/api/v1/system/version", "Check for updates", "System", mw...),
		h.Version,
	)

	// Self-update — nur Admin
	huma.Register(
		api,
		shared.Op("system-update", "POST", "/api/v1/admin/system/update", "Update Tidefly Plane", "System", adminMw...),
		h.UpdateSelf,
	)
}

func (h *Handler) RegisterSSERoutes(e *echo.Echo, echoSSE echo.MiddlewareFunc, echoInject echo.MiddlewareFunc) {
	e.GET(
		"/api/v1/system/caddy-logs", func(c *echo.Context) error {
			return h.CaddyLogs(c)
		}, echoSSE, echoInject,
	)

	// Update progress stream — Frontend subscribed nach POST /admin/system/update
	e.GET(
		"/api/v1/system/update-progress", func(c *echo.Context) error {
			return h.UpdateProgress(c)
		}, echoSSE, echoInject,
	)
}
