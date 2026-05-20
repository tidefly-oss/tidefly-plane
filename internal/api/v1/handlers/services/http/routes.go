package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("services-list", "GET", "/api/v1/services", "List services", "Services", mw...),
		h.ListServices,
	)
	huma.Register(
		api,
		shared.Op("services-create", "POST", "/api/v1/services", "Deploy a service", "Services", mw...),
		h.CreateService,
	)
	huma.Register(
		api,
		shared.Op("services-from-template", "POST", "/api/v1/services/from-template", "Deploy from template", "Services", mw...),
		h.CreateServiceFromTemplate,
	)
	huma.Register(
		api,
		shared.Op("services-get", "GET", "/api/v1/services/{id}", "Get service", "Services", mw...),
		h.GetService,
	)
	huma.Register(
		api,
		shared.Op("services-update", "PATCH", "/api/v1/services/{id}", "Update service", "Services", mw...),
		h.UpdateService,
	)
	huma.Register(
		api,
		shared.Op("services-delete", "DELETE", "/api/v1/services/{id}", "Delete service", "Services", mw...),
		h.DeleteService,
	)
	huma.Register(
		api,
		shared.Op("services-redeploy", "POST", "/api/v1/services/{id}/redeploy", "Redeploy service", "Services", mw...),
		h.RedeployService,
	)
}
