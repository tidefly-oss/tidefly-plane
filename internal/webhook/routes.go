package webhook

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, r chi.Router, mw huma.Middlewares) {
	huma.Register(api, httpx.Op("webhooks-list", "GET", httpx.V1+"/projects/{pid}/webhooks", "List webhooks", "Webhooks", mw...), h.list)
	huma.Register(api, httpx.Op("webhooks-create", "POST", httpx.V1+"/projects/{pid}/webhooks", "Create webhook", "Webhooks", mw...), h.create)
	huma.Register(api, httpx.Op("webhooks-get", "GET", httpx.V1+"/projects/{pid}/webhooks/{id}", "Get webhook", "Webhooks", mw...), h.get)
	huma.Register(api, httpx.Op("webhooks-update", "PATCH", httpx.V1+"/projects/{pid}/webhooks/{id}", "Update webhook", "Webhooks", mw...), h.update)
	huma.Register(api, httpx.Op("webhooks-delete", "DELETE", httpx.V1+"/projects/{pid}/webhooks/{id}", "Delete webhook", "Webhooks", mw...), h.delete)
	huma.Register(api, httpx.Op("webhooks-rotate", "POST", httpx.V1+"/projects/{pid}/webhooks/{id}/rotate", "Rotate secret", "Webhooks", mw...), h.rotateSecret)
	huma.Register(api, httpx.Op("webhooks-deliveries", "GET", httpx.V1+"/projects/{pid}/webhooks/{id}/deliveries", "List deliveries", "Webhooks", mw...), h.deliveries)

	// kein Auth — public receiver
	r.Post("/webhooks/{id}", h.Receive)
}
