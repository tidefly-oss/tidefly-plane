package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	agentpb "github.com/tidefly-oss/tidefly-plane/internal/agent/proto"
)

// ── ListWorkerContainers ──────────────────────────────────────────────────────

type listWorkerContainersInput struct {
	ID string `path:"id"`
}

type containerInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Status  string            `json:"status"`
	State   string            `json:"state"`
	Created int64             `json:"created"`
	Labels  map[string]string `json:"labels"`
}

type listWorkerContainersOutput struct {
	Body []containerInfo
}

func (h *Handler) listWorkerContainers(ctx context.Context, input *listWorkerContainersInput) (*listWorkerContainersOutput, error) {
	if !h.agentClient.IsConnected(input.ID) {
		return nil, huma.Error404NotFound("worker not connected")
	}
	containers, err := h.agentClient.ListContainers(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list containers: " + err.Error())
	}
	result := make([]containerInfo, 0, len(containers))
	for _, c := range containers {
		result = append(result, protoToContainerInfo(c))
	}
	return &listWorkerContainersOutput{Body: result}, nil
}

func protoToContainerInfo(c *agentpb.Container) containerInfo {
	return containerInfo{
		ID:      c.Id,
		Name:    c.Name,
		Image:   c.Image,
		Status:  c.Status,
		State:   c.State,
		Created: c.Created,
		Labels:  c.Labels,
	}
}

// ── WorkerContainerLogs (SSE) ─────────────────────────────────────────────────

// WorkerContainerLogs streams logs from a container on a worker node via
// the gRPC tunnel, forwarded as SSE to the browser.
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
