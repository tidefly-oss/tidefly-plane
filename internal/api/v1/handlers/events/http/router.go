package http

import (
	"github.com/labstack/echo/v5"
)

func (h *Handler) RegisterRoutes(e *echo.Echo, echoAuth, echoInject echo.MiddlewareFunc) {
	e.GET("/api/v1/events/stream", h.Stream, echoAuth, echoInject)
}
