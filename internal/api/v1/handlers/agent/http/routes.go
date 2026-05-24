package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(
	api huma.API,
	r chi.Router,
	mw huma.Middlewares,
	adminMw huma.Middlewares,
	sseAuth func(http.Handler) http.Handler,
) {
	// ── Public ────────────────────────────────────────────────────────────────
	huma.Register(api, shared.Op("agent-register", http.MethodPost, "/api/v1/agent/register", "Register a worker agent", "Agent"), h.Register)

	// ── mTLS authenticated ────────────────────────────────────────────────────
	huma.Register(api, shared.Op("agent-renew-cert", http.MethodPost, "/api/v1/agent/renew", "Renew worker mTLS certificate", "Agent", mw...), h.RenewCert)

	// ── Admin only ────────────────────────────────────────────────────────────
	huma.Register(api, shared.Op("agent-create-token", http.MethodPost, "/api/v1/agent/tokens", "Create a worker registration token", "Agent", adminMw...), h.CreateToken)
	huma.Register(api, shared.Op("agent-list-tokens", http.MethodGet, "/api/v1/agent/tokens", "List registration tokens", "Agent", adminMw...), h.ListTokens)
	huma.Register(api, shared.Op("agent-list-workers", http.MethodGet, "/api/v1/agent/workers", "List registered worker nodes", "Agent", mw...), h.ListWorkers)
	huma.Register(api, shared.Op("agent-revoke-worker", http.MethodDelete, "/api/v1/agent/workers/{id}", "Revoke a worker node", "Agent", adminMw...), h.RevokeWorker)
	huma.Register(api, shared.Op("agent-delete-worker", http.MethodDelete, "/api/v1/agent/workers/{id}/permanent", "Permanently delete a revoked worker node", "Agent", adminMw...), h.DeleteWorker)

	// ── Worker containers ─────────────────────────────────────────────────────
	huma.Register(api, shared.Op("agent-list-worker-containers", http.MethodGet, "/api/v1/agent/workers/{id}/containers", "List containers on a worker node", "Agent", mw...), h.ListWorkerContainers)

	// ── SSE — Worker container logs via gRPC tunnel ───────────────────────────
	r.With(sseAuth).Get("/api/v1/agent/workers/{id}/containers/{containerID}/logs", h.WorkerContainerLogs)
}
