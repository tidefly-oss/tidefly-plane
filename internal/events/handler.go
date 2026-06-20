package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
)

type Handler struct {
	rt runtime.Runtime
}

func NewHandler(rt runtime.Runtime) *Handler {
	return &Handler{rt: rt}
}

func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

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

	sendEvent := func(event, data string) {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	eventCh, errCh := h.rt.EventStream(ctx)
	sendEvent("ping", "connected")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				sendEvent("error", `{"message":"runtime event stream disconnected"}`)
			}
			return
		case evt, ok := <-eventCh:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			sendEvent("container", string(data))
		case <-ticker.C:
			sendEvent("ping", "keepalive")
		}
	}
}
