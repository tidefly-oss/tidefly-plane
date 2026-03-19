package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

func (h *Handler) StreamAppLogs(c *echo.Context) error {
	level := c.QueryParam("level")
	component := c.QueryParam("component")

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

	ctx := c.Request().Context()
	lastID := h.logs.LatestAppLogID()

	heartbeat := time.NewTicker(15 * time.Second)
	poll := time.NewTicker(2 * time.Second)
	defer heartbeat.Stop()
	defer poll.Stop()

	sendEvent := func(event, data string) {
		_, _ = fmt.Fprintf(resp, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(resp, ": heartbeat\n\n")
			flusher.Flush()
		case <-poll.C:
			newLogs, err := h.logs.PollAppLogs(lastID, level, component)
			if err != nil {
				continue
			}
			for _, entry := range newLogs {
				data, _ := json.Marshal(entry)
				sendEvent("log", string(data))
				if entry.ID > lastID {
					lastID = entry.ID
				}
			}
		}
	}
}
