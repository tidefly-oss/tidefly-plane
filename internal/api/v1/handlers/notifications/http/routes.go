package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	e *echo.Echo,
	mw huma.Middlewares,
	echoAuth, echoInject echo.MiddlewareFunc,
) {
	huma.Register(api, shared.Op("notif-list", "GET", "/api/v1/notifications", "List notifications", mw...), h.List)
	huma.Register(
		api,
		shared.Op("notif-list-all", "GET", "/api/v1/notifications/all", "All notifications", mw...),
		h.ListAll,
	)
	huma.Register(
		api,
		shared.Op("notif-count", "GET", "/api/v1/notifications/count", "Notification count", mw...),
		h.Count,
	)
	huma.Register(
		api,
		shared.Op("notif-ack", "POST", "/api/v1/notifications/{id}/acknowledge", "Acknowledge", mw...),
		h.Acknowledge,
	)
	huma.Register(
		api,
		shared.Op("notif-delete", "DELETE", "/api/v1/notifications/{id}", "Delete notification", mw...),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op("notif-delete-acked", "DELETE", "/api/v1/notifications/acknowledged", "Delete acknowledged", mw...),
		h.DeleteAcknowledged,
	)
	e.GET("/api/v1/notifications/stream", h.Stream, echoAuth, echoInject)
}
