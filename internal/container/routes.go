package container

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	r chi.Router,
	mw huma.Middlewares,
	sseAuth func(http.Handler) http.Handler,
) {
	// ── Huma ──────────────────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("containers-list", "GET", httpx.V1+"/containers", "List containers", "Containers", mw...), h.list)
	huma.Register(api, httpx.Op("containers-get", "GET", httpx.V1+"/containers/{id}", "Get container", "Containers", mw...), h.get)
	huma.Register(api, httpx.Op("containers-start", "POST", httpx.V1+"/containers/{id}/start", "Start container", "Containers", mw...), h.start)
	huma.Register(api, httpx.Op("containers-stop", "POST", httpx.V1+"/containers/{id}/stop", "Stop container", "Containers", mw...), h.stop)
	huma.Register(api, httpx.Op("containers-restart", "POST", httpx.V1+"/containers/{id}/restart", "Restart container", "Containers", mw...), h.restart)
	huma.Register(api, httpx.Op("containers-get-resources", "GET", httpx.V1+"/containers/{id}/resources", "Get resource limits", "Containers", mw...), h.getResources)
	huma.Register(api, httpx.Op("containers-update-resources", "PATCH", httpx.V1+"/containers/{id}/resources", "Update resource limits", "Containers", mw...), h.updateResources)

	// ── SSE / WebSocket ───────────────────────────────────────────────────────
	r.With(sseAuth).Get(httpx.V1+"/containers/{id}/logs", h.Logs)
	r.With(sseAuth).Get(httpx.V1+"/containers/{id}/stats", h.Stats)
	r.With(sseAuth).Get(httpx.V1+"/containers/{id}/exec", h.Exec)
}
