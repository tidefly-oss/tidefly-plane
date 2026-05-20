package http

import "github.com/labstack/echo/v5"

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.GET("/api/v1/ws", h.ServeWS)
}
