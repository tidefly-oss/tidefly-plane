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
	huma.Register(
		api,
		shared.Op("notif-list", "GET", "/api/v1/notifications", "List notifications", "Notifications", mw...),
		h.List,
	)
	huma.Register(
		api,
		shared.Op("notif-list-all", "GET", "/api/v1/notifications/all", "All notifications", "Notifications", mw...),
		h.ListAll,
	)
	huma.Register(
		api,
		shared.Op("notif-count", "GET", "/api/v1/notifications/count", "Notification count", "Notifications", mw...),
		h.Count,
	)
	huma.Register(
		api,
		shared.Op("notif-ack", "POST", "/api/v1/notifications/{id}/acknowledge", "Acknowledge", "Notifications", mw...),
		h.Acknowledge,
	)
	huma.Register(
		api,
		shared.Op(
			"notif-delete",
			"DELETE",
			"/api/v1/notifications/{id}",
			"Delete notification",
			"Notifications",
			mw...,
		),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op(
			"notif-delete-acked",
			"DELETE",
			"/api/v1/notifications/acknowledged",
			"Delete acknowledged",
			"Notifications",
			mw...,
		),
		h.DeleteAcknowledged,
	)
	e.GET("/api/v1/notifications/stream", h.Stream, echoAuth, echoInject)
}
