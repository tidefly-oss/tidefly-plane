package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/api/shared"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	e *echo.Echo,
	mw huma.Middlewares,
	adminMw huma.Middlewares,
	echoAuth, echoInject echo.MiddlewareFunc,
) {
	huma.Register(api, shared.Op("logs-app", "GET", "/api/v1/logs/app", "App logs", mw...), h.ListAppLogs)
	huma.Register(api, shared.Op("logs-audit", "GET", "/api/v1/logs/audit", "Audit logs", adminMw...), h.ListAuditLogs)
	e.GET("/api/v1/logs/app/stream", h.StreamAppLogs, echoAuth, echoInject)
}
