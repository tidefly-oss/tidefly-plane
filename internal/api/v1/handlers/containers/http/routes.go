package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-backend/internal/api/shared"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	e *echo.Echo,
	mw huma.Middlewares,
	echoAuth, echoInject echo.MiddlewareFunc,
) {
	// ── Huma ──────────────────────────────────────────────────────────────────
	huma.Register(api, shared.Op("containers-list", "GET", "/api/v1/containers", "List containers", mw...), h.List)
	huma.Register(api, shared.Op("containers-get", "GET", "/api/v1/containers/{id}", "Get container", mw...), h.Get)
	huma.Register(
		api,
		shared.Op("containers-start", "POST", "/api/v1/containers/{id}/start", "Start container", mw...),
		h.Start,
	)
	huma.Register(
		api,
		shared.Op("containers-stop", "POST", "/api/v1/containers/{id}/stop", "Stop container", mw...),
		h.Stop,
	)
	huma.Register(
		api,
		shared.Op("containers-restart", "POST", "/api/v1/containers/{id}/restart", "Restart container", mw...),
		h.Restart,
	)
	huma.Register(
		api,
		shared.Op("containers-delete", "DELETE", "/api/v1/containers/{id}", "Delete container", mw...),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op("containers-get-resources", "GET", "/api/v1/containers/{id}/resources", "Get resource limits", mw...),
		h.GetResources,
	)
	huma.Register(
		api,
		shared.Op(
			"containers-update-resources",
			"PATCH",
			"/api/v1/containers/{id}/resources",
			"Update resource limits",
			mw...,
		),
		h.UpdateResources,
	)
	huma.Register(
		api,
		shared.Op("containers-compose", "POST", "/api/v1/containers/compose", "Deploy Compose stack", mw...),
		h.DeployCompose,
	)
	huma.Register(
		api,
		shared.Op("containers-delete-stack", "DELETE", "/api/v1/containers/stacks/{stack_id}", "Delete stack", mw...),
		h.DeleteStack,
	)

	// ── Echo SSE/WS ───────────────────────────────────────────────────────────
	e.GET("/api/v1/containers/:id/logs", h.Logs, echoAuth, echoInject)
	e.GET("/api/v1/containers/:id/stats", h.Stats, echoAuth, echoInject)
	e.GET("/api/v1/containers/:id/exec", h.Exec, echoAuth, echoInject)
	e.POST("/api/v1/containers/dockerfile", h.BuildAndDeploy, echoAuth, echoInject)
}

// ensure middleware is used
var _ = middleware.CheckContainerAccess
