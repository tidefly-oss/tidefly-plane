package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	e *echo.Echo,
	mw huma.Middlewares,
	echoAuth, echoInject echo.MiddlewareFunc,
) {
	// ── Huma ──────────────────────────────────────────────────────────────────
	huma.Register(api, shared.Op("containers-list", "GET", "/api/v1/containers", "List containers", "Containers", mw...), h.List)
	huma.Register(api, shared.Op("containers-get", "GET", "/api/v1/containers/{id}", "Get container", "Containers", mw...), h.Get)
	huma.Register(api, shared.Op("containers-start", "POST", "/api/v1/containers/{id}/start", "Start container", "Containers", mw...), h.Start)
	huma.Register(api, shared.Op("containers-stop", "POST", "/api/v1/containers/{id}/stop", "Stop container", "Containers", mw...), h.Stop)
	huma.Register(api, shared.Op("containers-restart", "POST", "/api/v1/containers/{id}/restart", "Restart container", "Containers", mw...), h.Restart)
	huma.Register(api, shared.Op("containers-get-resources", "GET", "/api/v1/containers/{id}/resources", "Get resource limits", "Containers", mw...), h.GetResources)
	huma.Register(api, shared.Op("containers-update-resources", "PATCH", "/api/v1/containers/{id}/resources", "Update resource limits", "Containers", mw...), h.UpdateResources)

	// ── Echo SSE/WS ───────────────────────────────────────────────────────────
	e.GET("/api/v1/containers/:id/logs", h.Logs, echoAuth, echoInject)
	e.GET("/api/v1/containers/:id/stats", h.Stats, echoAuth, echoInject)
	e.GET("/api/v1/containers/:id/exec", h.Exec, echoAuth, echoInject)
}

var _ = middleware.CheckContainerAccess
