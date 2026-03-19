package http

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func (h *Handler) Exec(c *echo.Context) error {
	id := c.Param("id")
	details, err := h.runtime.GetContainer(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "container not found"})
	}
	if err := h.access.CheckContainerAccess(c, details.Labels); err != nil {
		return err
	}
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := ws.Close(); err != nil {
			h.log.Error("exec", "failed to close websocket", err)
		}
	}()
	return h.runtime.ExecAttach(c.Request().Context(), id, ws)
}
