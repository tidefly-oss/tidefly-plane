package events

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) RegisterSSERoutes(r chi.Router, sseAuth func(http.Handler) http.Handler) {
	r.With(sseAuth).Get("/api/v1/events/stream", h.Stream)
}
