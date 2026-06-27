package manifest

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, httpx.Op("manifest-list", "GET", httpx.V1+"/manifest", "List manifest", "Manifest", mw...), h.list)
	huma.Register(api, httpx.Op("manifest-create", "POST", httpx.V1+"/manifest", "Deploy a service", "Manifest", mw...), h.create)
	huma.Register(api, httpx.Op("manifest-from-template", "POST", httpx.V1+"/manifest/from-template", "Deploy from template", "Manifest", mw...), h.createFromTemplate)
	huma.Register(api, httpx.Op("manifest-get", "GET", httpx.V1+"/manifest/{id}", "Get service", "Manifest", mw...), h.get)
	huma.Register(api, httpx.Op("manifest-update", "PATCH", httpx.V1+"/manifest/{id}", "Update service", "Manifest", mw...), h.update)
	huma.Register(api, httpx.Op("manifest-delete", "DELETE", httpx.V1+"/manifest/{id}", "Delete service", "Manifest", mw...), h.delete)
	huma.Register(api, httpx.Op("manifest-redeploy", "POST", httpx.V1+"/manifest/{id}/redeploy", "Redeploy service", "Manifest", mw...), h.redeploy)
}
