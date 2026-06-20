package agent

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
	adminMw huma.Middlewares,
	sseAuth func(http.Handler) http.Handler,
) {
	// ── Public ────────────────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("agent-register", http.MethodPost, httpx.V1+"/agent/register", "Register a worker agent", "Agent"), h.register)

	// ── mTLS authenticated ────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("agent-renew-cert", http.MethodPost, httpx.V1+"/agent/renew", "Renew worker mTLS certificate", "Agent", mw...), h.renewCert)

	// ── Admin only ────────────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("agent-create-token", http.MethodPost, httpx.V1+"/agent/tokens", "Create a worker registration token", "Agent", adminMw...), h.createToken)
	huma.Register(api, httpx.Op("agent-list-tokens", http.MethodGet, httpx.V1+"/agent/tokens", "List registration tokens", "Agent", adminMw...), h.listTokens)
	huma.Register(api, httpx.Op("agent-list-workers", http.MethodGet, httpx.V1+"/agent/workers", "List registered worker nodes", "Agent", mw...), h.listWorkers)
	huma.Register(api, httpx.Op("agent-revoke-worker", http.MethodDelete, httpx.V1+"/agent/workers/{id}", "Revoke a worker node", "Agent", adminMw...), h.revokeWorker)
	huma.Register(api, httpx.Op("agent-delete-worker", http.MethodDelete, httpx.V1+"/agent/workers/{id}/permanent", "Permanently delete a revoked worker node", "Agent", adminMw...), h.deleteWorker)

	// ── Worker containers ─────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("agent-list-worker-containers", http.MethodGet, httpx.V1+"/agent/workers/{id}/containers", "List containers on a worker node", "Agent", mw...), h.listWorkerContainers)

	// ── SSE — Worker container logs via gRPC tunnel ───────────────────────────
	r.With(sseAuth).Get(httpx.V1+"/agent/workers/{id}/containers/{containerID}/logs", h.WorkerContainerLogs)
}
