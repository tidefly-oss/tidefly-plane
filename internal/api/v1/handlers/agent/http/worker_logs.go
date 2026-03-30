package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
)

// WorkerContainerLogs streams logs from a container running on a worker
// via the gRPC tunnel, forwarded as SSE to the browser.
// GET /api/v1/agent/workers/:id/containers/:containerID/logs
func (h *Handler) WorkerContainerLogs(c *echo.Context) error {
	workerID := c.Param("id")
	containerID := c.Param("containerID")

	tail, _ := strconv.ParseInt(c.QueryParam("tail"), 10, 32)
	if tail == 0 {
		tail = 100
	}
	follow := c.QueryParam("follow") != "false"

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

	if !h.agentClient.IsConnected(workerID) {
		data, _ := json.Marshal(map[string]string{"error": "worker not connected"})
		_, _ = fmt.Fprintf(resp, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
		return nil
	}

	// Use agent client to stream logs via gRPC tunnel
	agentClient := h.agentClient
	logCh, err := agentClient.StreamLogs(ctx, workerID, containerID, follow, int32(tail))
	if err != nil {
		data, _ := json.Marshal(map[string]string{"error": err.Error()})
		_, _ = fmt.Fprintf(resp, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-logCh:
			if !ok {
				_, _ = fmt.Fprintf(resp, "event: done\ndata: {}\n\n")
				flusher.Flush()
				return nil
			}
			streamType := "stdout"
			if msg.IsStderr {
				streamType = "stderr"
			}
			data, _ := json.Marshal(
				map[string]string{
					"stream": streamType,
					"line":   msg.Line,
				},
			)
			_, _ = fmt.Fprintf(resp, "event: log\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

var _ = middleware.CheckContainerAccess
