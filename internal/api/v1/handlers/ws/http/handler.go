package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/olahol/melody"

	"github.com/tidefly-oss/tidefly-plane/internal/domain/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

type Handler struct {
	bus     *eventbus.Bus
	jwtSvc  *auth.Service
	log     *logger.Logger
	metrics *metrics.Registry
}

func New(bus *eventbus.Bus, jwtSvc *auth.Service, log *logger.Logger, reg *metrics.Registry) *Handler {
	h := &Handler{bus: bus, jwtSvc: jwtSvc, log: log, metrics: reg}
	h.setupHandlers()
	return h
}

// clientMsg is the shape of messages sent from the UI to Plane.
type clientMsg struct {
	Type   string   `json:"type"`
	Topics []string `json:"topics,omitempty"`
}

func (h *Handler) setupHandlers() {
	m := h.bus.Melody()
	m.HandleConnect(func(s *melody.Session) {
		h.bus.SetTopics(s, []string{"*"})
		go func() {
			time.Sleep(100 * time.Millisecond)
			snap := h.metrics.GetSystem()
			evt := eventbus.Event{
				Type:  eventbus.EventSystemMetrics,
				Topic: eventbus.TopicMetrics,
				Payload: eventbus.SystemMetricsPayload{
					CPUPercent: snap.CPUPercent,
					MemPercent: snap.MemPercent,
					DiskUsed:   snap.DiskUsedMB,
					DiskTotal:  snap.DiskTotalMB,
				},
			}
			data, err := json.Marshal(evt)
			if err == nil {
				_ = s.Write(data)
			}
		}()
	})
	m.HandleMessage(func(s *melody.Session, msg []byte) {
		var cm clientMsg
		if err := json.Unmarshal(msg, &cm); err != nil {
			return
		}
		switch cm.Type {
		case "subscribe":
			if len(cm.Topics) > 0 {
				h.bus.SetTopics(s, cm.Topics)
			}
		case "ping":
			pong, _ := json.Marshal(map[string]string{"type": "pong"})
			_ = s.Write(pong)
		}
	})
	m.HandleError(func(s *melody.Session, err error) {
		h.log.Warnw("ws error", "err", err)
	})
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, `{"message":"missing token"}`, http.StatusUnauthorized)
		return
	}
	if _, err := h.jwtSvc.ValidateAccessToken(token); err != nil {
		http.Error(w, `{"message":"invalid token"}`, http.StatusUnauthorized)
		return
	}
	if err := h.bus.Melody().HandleRequest(w, r); err != nil {
		h.log.Errorw("ws", "websocket upgrade failed", "error", err)
	}
}
