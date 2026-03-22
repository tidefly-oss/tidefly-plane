package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"
	caddysvc "github.com/tidefly-oss/tidefly-backend/internal/services/caddy"
)

func (h *Handler) CaddyLogs(c *echo.Context) error {
	ctx := (*c).Request().Context()
	resp := (*c).Response()

	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)

	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	sendEvent := func(data []byte) {
		_, _ = fmt.Fprintf(resp, "data: %s\n\n", data)
		flusher.Flush()
	}

	logWatcher := caddysvc.NewLogWatcher(h.runtime, h.log)

	if tailStr := (*c).QueryParam("tail"); tailStr != "" {
		if n, err := strconv.Atoi(tailStr); err == nil && n > 0 {
			if entries, err := logWatcher.Tail(ctx, n); err == nil {
				for _, entry := range entries {
					data, _ := json.Marshal(entry)
					sendEvent(data)
				}
			}
		}
	}

	ch, err := logWatcher.Stream(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(resp, "event: error\ndata: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
		return nil
	}

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case entry, ok := <-ch:
			if !ok {
				return nil
			}
			data, _ := json.Marshal(entry)
			sendEvent(data)
		case <-ticker.C:
			_, _ = fmt.Fprintf(resp, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}
