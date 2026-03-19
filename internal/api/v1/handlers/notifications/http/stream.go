package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

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
		_, _ = fmt.Fprintf(resp, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	ch, unsub := h.svc.Subscribe(ctx)
	defer unsub()

	sendEvent("ping", "connected")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case data, ok := <-ch:
			if !ok {
				return nil
			}
			sendEvent("notification", string(data))
		case <-ticker.C:
			sendEvent("ping", "keepalive")
		}
	}
}
