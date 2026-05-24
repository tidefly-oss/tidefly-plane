package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (h *Handler) StreamAppLogs(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	component := r.URL.Query().Get("component")

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
	lastID := h.logs.LatestAppLogID()
	heartbeat := time.NewTicker(15 * time.Second)
	poll := time.NewTicker(2 * time.Second)
	defer heartbeat.Stop()
	defer poll.Stop()

	sendEvent := func(event, data string) {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
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
