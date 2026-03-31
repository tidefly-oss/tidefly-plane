package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("projects-list", "GET", "/api/v1/projects", "List projects", "Projects", mw...),
		h.List,
	)
	huma.Register(
		api,
		shared.Op("projects-create", "POST", "/api/v1/projects", "Create project", "Projects", mw...),
		h.Create,
	)
	huma.Register(
		api,
		shared.Op("projects-get", "GET", "/api/v1/projects/{id}", "Get project", "Projects", mw...),
		h.Get,
	)
	huma.Register(
		api,
		shared.Op("projects-update", "PUT", "/api/v1/projects/{id}", "Update project", "Projects", mw...),
		h.Update,
	)
	huma.Register(
		api,
		shared.Op("projects-delete", "DELETE", "/api/v1/projects/{id}", "Delete project", "Projects", mw...),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op(
			"projects-containers",
			"GET",
			"/api/v1/projects/{id}/containers",
			"Project containers",
			"Projects",
			mw...,
		),
		h.ListContainers,
	)
}
