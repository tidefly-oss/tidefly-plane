package ws

import (
	"github.com/go-chi/chi/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get(httpx.V1+"/ws", h.ServeWS)
}
