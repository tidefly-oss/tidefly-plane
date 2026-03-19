package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, e *echo.Echo, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("webhooks-list", "GET", "/api/v1/projects/{pid}/webhooks", "List webhooks", mw...),
		h.List,
	)
	huma.Register(
		api,
		shared.Op("webhooks-create", "POST", "/api/v1/projects/{pid}/webhooks", "Create webhook", mw...),
		h.Create,
	)
	huma.Register(
		api,
		shared.Op("webhooks-get", "GET", "/api/v1/projects/{pid}/webhooks/{id}", "Get webhook", mw...),
		h.Get,
	)
	huma.Register(
		api,
		shared.Op("webhooks-update", "PATCH", "/api/v1/projects/{pid}/webhooks/{id}", "Update webhook", mw...),
		h.Update,
	)
	huma.Register(
		api,
		shared.Op("webhooks-delete", "DELETE", "/api/v1/projects/{pid}/webhooks/{id}", "Delete webhook", mw...),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op("webhooks-rotate", "POST", "/api/v1/projects/{pid}/webhooks/{id}/rotate", "Rotate secret", mw...),
		h.RotateSecret,
	)
	huma.Register(
		api,
		shared.Op(
			"webhooks-deliveries",
			"GET",
			"/api/v1/projects/{pid}/webhooks/{id}/deliveries",
			"List deliveries",
			mw...,
		),
		h.Deliveries,
	)

	// kein Auth — public receiver
	e.POST("/webhooks/:id", h.Receive)
}
