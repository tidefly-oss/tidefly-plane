package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, e *echo.Echo, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("webhooks-list", "GET", "/api/v1/projects/{pid}/webhooks", "List webhooks", "Webhooks", mw...),
		h.List,
	)
	huma.Register(
		api,
		shared.Op("webhooks-create", "POST", "/api/v1/projects/{pid}/webhooks", "Create webhook", "Webhooks", mw...),
		h.Create,
	)
	huma.Register(
		api,
		shared.Op("webhooks-get", "GET", "/api/v1/projects/{pid}/webhooks/{id}", "Get webhook", "Webhooks", mw...),
		h.Get,
	)
	huma.Register(
		api,
		shared.Op(
			"webhooks-update",
			"PATCH",
			"/api/v1/projects/{pid}/webhooks/{id}",
			"Update webhook",
			"Webhooks",
			mw...,
		),
		h.Update,
	)
	huma.Register(
		api,
		shared.Op(
			"webhooks-delete",
			"DELETE",
			"/api/v1/projects/{pid}/webhooks/{id}",
			"Delete webhook",
			"Webhooks",
			mw...,
		),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op(
			"webhooks-rotate",
			"POST",
			"/api/v1/projects/{pid}/webhooks/{id}/rotate",
			"Rotate secret",
			"Webhooks",
			mw...,
		),
		h.RotateSecret,
	)
	huma.Register(
		api,
		shared.Op(
			"webhooks-deliveries",
			"GET",
			"/api/v1/projects/{pid}/webhooks/{id}/deliveries",
			"List deliveries",
			"Webhooks",
			mw...,
		),
		h.Deliveries,
	)
	// kein Auth — public receiver
	e.POST("/webhooks/:id", h.Receive)
}
