package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, shared.Op("system-health", "GET", "/api/v1/system/health", "Health check", mw...), h.Health)
	huma.Register(api, shared.Op("system-info", "GET", "/api/v1/system/info", "Runtime info", mw...), h.Info)
	huma.Register(
		api,
		shared.Op("system-overview", "GET", "/api/v1/system/overview", "Dashboard overview", mw...),
		h.Overview,
	)
	huma.Register(
		api,
		shared.Op("system-metrics", "GET", "/api/v1/system/metrics", "Current system metrics", mw...),
		h.Metrics,
	)
	huma.Register(api, shared.Op("system-ports", "GET", "/api/v1/system/ports", "Used host ports", mw...), h.UsedPorts)
}

func (h *Handler) RegisterSSERoutes(e *echo.Echo, echoSSE echo.MiddlewareFunc, echoInject echo.MiddlewareFunc) {
	e.GET(
		"/api/v1/system/caddy-logs", func(c *echo.Context) error {
			return h.CaddyLogs(c)
		}, echoSSE, echoInject,
	)
}
