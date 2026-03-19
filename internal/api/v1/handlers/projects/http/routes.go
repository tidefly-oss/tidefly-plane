package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-backend/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, shared.Op("projects-list", "GET", "/api/v1/projects", "List projects", mw...), h.List)
	huma.Register(api, shared.Op("projects-create", "POST", "/api/v1/projects", "Create project", mw...), h.Create)
	huma.Register(api, shared.Op("projects-get", "GET", "/api/v1/projects/{id}", "Get project", mw...), h.Get)
	huma.Register(api, shared.Op("projects-update", "PUT", "/api/v1/projects/{id}", "Update project", mw...), h.Update)
	huma.Register(
		api,
		shared.Op("projects-delete", "DELETE", "/api/v1/projects/{id}", "Delete project", mw...),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op("projects-containers", "GET", "/api/v1/projects/{id}/containers", "Project containers", mw...),
		h.ListContainers,
	)
}
