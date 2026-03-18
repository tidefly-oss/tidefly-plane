package containers

// Exec bleibt auf rohem Echo wegen WebSocket-Upgrade.

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
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
	if err := middleware.CheckContainerAccess(c, h.db, details.Labels); err != nil {
		return err
	}
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	return h.runtime.ExecAttach(c.Request().Context(), id, ws)
}
