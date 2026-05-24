package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
)

// WorkerContainerLogs streams logs from a container running on a worker
// via the gRPC tunnel, forwarded as SSE to the browser.
// GET /api/v1/agent/workers/{id}/containers/{containerID}/logs
func (h *Handler) WorkerContainerLogs(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")
	containerID := chi.URLParam(r, "containerID")

	tail, _ := strconv.ParseInt(r.URL.Query().Get("tail"), 10, 32)
	if tail == 0 {
		tail = 100
	}
	follow := r.URL.Query().Get("follow") != "false"

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	if !h.agentClient.IsConnected(workerID) {
		data, _ := json.Marshal(map[string]string{"error": "worker not connected"})
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
		return
	}

	logCh, err := h.agentClient.StreamLogs(ctx, workerID, containerID, follow, int32(tail))
	if err != nil {
		data, _ := json.Marshal(map[string]string{"error": err.Error()})
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-logCh:
			if !ok {
				_, _ = fmt.Fprintf(w, "event: done\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			streamType := "stdout"
			if msg.IsStderr {
				streamType = "stderr"
			}
			data, _ := json.Marshal(map[string]string{
				"stream": streamType,
				"line":   msg.Line,
			})
			_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

var _ = middleware.CheckContainerAccess
