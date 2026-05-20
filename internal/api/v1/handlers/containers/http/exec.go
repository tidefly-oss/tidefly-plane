package http

import (
	"net/http"
	"sync"

	"github.com/labstack/echo/v5"
	"github.com/olahol/melody"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
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
				h.log.Warnw("exec attach error", "err", err)
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
		h.log.Warnw("exec ws error", "err", err)
	})
}

func (h *Handler) Exec(c *echo.Context) error {
	id := c.Param("id")
	details, err := h.runtime.GetContainer(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "container not found"})
	}
	if err := h.access.CheckContainerAccess(c, details.Labels); err != nil {
		return err
	}

	// Inject container ID as query param so melody's HandleConnect can read it
	req := c.Request()
	q := req.URL.Query()
	q.Set("id", id)
	req.URL.RawQuery = q.Encode()

	w, err := echo.UnwrapResponse(c.Response())
	if err != nil {
		return err
	}
	return h.execMel.HandleRequest(w, req)
}
