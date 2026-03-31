package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("deploy-list", "GET", "/api/v1/deploy", "List services", "Deploy", mw...),
		h.ListServices,
	)
	huma.Register(
		api,
		shared.Op("deploy-create", "POST", "/api/v1/deploy", "Deploy service", "Deploy", mw...),
		h.DeployService,
	)
	huma.Register(
		api,
		shared.Op("deploy-delete", "DELETE", "/api/v1/deploy/{id}", "Delete service", "Deploy", mw...),
		h.DeleteService,
	)
	huma.Register(
		api,
		shared.Op(
			"deploy-credentials-shown",
			"POST",
			"/api/v1/deploy/{id}/credentials/shown",
			"Mark credentials shown",
			"Deployments",
			mw...,
		),
		h.MarkCredentialsShown,
	)
}
