package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
)

func (h *Handler) CaddyLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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

	sendEvent := func(data []byte) {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	logWatcher := caddysvc.NewLogWatcher(h.runtime, h.log)

	if tailStr := r.URL.Query().Get("tail"); tailStr != "" {
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
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
		return
	}

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(entry)
			sendEvent(data)
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}
