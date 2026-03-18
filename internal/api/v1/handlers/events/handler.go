package events

// Stream bleibt auf rohem Echo — SSE ist mit Huma nicht kompatibel.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

type Handler struct {
	rt runtime.Runtime
}

func New(rt runtime.Runtime) *Handler {
	return &Handler{rt: rt}
}

func (h *Handler) Stream(c *echo.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-c.Request().Context().Done()
		cancel()
	}()

	resp := c.Response()
	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)

	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	sendEvent := func(event, data string) {
		fmt.Fprintf(resp, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	eventCh, errCh := h.rt.EventStream(ctx)
	sendEvent("ping", "connected")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			if err != nil {
				sendEvent("error", `{"message":"runtime event stream disconnected"}`)
			}
			return nil
		case evt, ok := <-eventCh:
			if !ok {
				return nil
			}
			data, _ := json.Marshal(evt)
			sendEvent("container", string(data))
		case <-ticker.C:
			sendEvent("ping", "keepalive")
		}
	}
}
