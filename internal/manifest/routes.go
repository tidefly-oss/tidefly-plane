package manifest

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, httpx.Op("services-list", "GET", httpx.V1+"/services", "List services", "Services", mw...), h.list)
	huma.Register(api, httpx.Op("services-create", "POST", httpx.V1+"/services", "Deploy a service", "Services", mw...), h.create)
	huma.Register(api, httpx.Op("services-from-template", "POST", httpx.V1+"/services/from-template", "Deploy from template", "Services", mw...), h.createFromTemplate)
	huma.Register(api, httpx.Op("services-get", "GET", httpx.V1+"/services/{id}", "Get service", "Services", mw...), h.get)
	huma.Register(api, httpx.Op("services-update", "PATCH", httpx.V1+"/services/{id}", "Update service", "Services", mw...), h.update)
	huma.Register(api, httpx.Op("services-delete", "DELETE", httpx.V1+"/services/{id}", "Delete service", "Services", mw...), h.delete)
	huma.Register(api, httpx.Op("services-redeploy", "POST", httpx.V1+"/services/{id}/redeploy", "Redeploy service", "Services", mw...), h.redeploy)
}
