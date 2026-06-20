package container

import (
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/olahol/melody"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
)

// melodyConn bridges a melody.Session to runtime.ExecConn.
type melodyConn struct {
	s      *melody.Session
	readCh chan melodyMsg
}

type melodyMsg struct {
	msgType int
	data    []byte
}

func newMelodyConn(s *melody.Session) *melodyConn {
	return &melodyConn{s: s, readCh: make(chan melodyMsg, 64)}
}

func (m *melodyConn) feed(msgType int, data []byte) {
	m.readCh <- melodyMsg{msgType: msgType, data: data}
}

func (m *melodyConn) ReadMessage() (int, []byte, error) {
	msg := <-m.readCh
	return msg.msgType, msg.data, nil
}

func (m *melodyConn) WriteMessage(msgType int, data []byte) error {
	if msgType == runtime.WSBinary {
		return m.s.WriteBinary(data)
	}
	return m.s.Write(data)
}

var execSessions sync.Map // *melody.Session → *melodyConn

func (h *Handler) setupExecHandlers() {
	h.execMel.HandleConnect(func(s *melody.Session) {
		ids, ok := s.Request.URL.Query()["id"]
		if !ok || len(ids) == 0 {
			_ = s.Close()
			return
		}
		containerID := ids[0]
		mc := newMelodyConn(s)
		execSessions.Store(s, mc)
		go func() {
			ctx := s.Request.Context()
			if err := h.runtime.ExecAttach(ctx, containerID, mc); err != nil {
				h.log.Info("exec", "attach ended: "+err.Error())
			}
			execSessions.Delete(s)
			_ = s.Close()
		}()
	})
	h.execMel.HandleMessage(func(s *melody.Session, msg []byte) {
		if v, ok := execSessions.Load(s); ok {
			v.(*melodyConn).feed(runtime.WSText, msg)
		}
	})
	h.execMel.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		if v, ok := execSessions.Load(s); ok {
			v.(*melodyConn).feed(runtime.WSBinary, msg)
		}
	})
	h.execMel.HandleDisconnect(func(s *melody.Session) {
		execSessions.Delete(s)
	})
	h.execMel.HandleError(func(s *melody.Session, err error) {
		// Normal client disconnect — not an error
		h.log.Info("exec", "client disconnected: "+err.Error())
	})
}

func (h *Handler) Exec(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	details, err := h.runtime.GetContainer(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"container not found"}`, http.StatusNotFound)
		return
	}
	if err := h.access.CheckContainerAccess(r.Context(), details.Labels); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	q := r.URL.Query()
	q.Set("id", id)
	r.URL.RawQuery = q.Encode()
	if err := h.execMel.HandleRequest(w, r); err != nil {
		h.log.Errorw("containers", "exec websocket upgrade failed", "error", err)
	}
}
