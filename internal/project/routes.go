package project

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, httpx.Op("projects-list", "GET", httpx.V1+"/projects", "List projects", "Projects", mw...), h.list)
	huma.Register(api, httpx.Op("projects-create", "POST", httpx.V1+"/projects", "Create project", "Projects", mw...), h.create)
	huma.Register(api, httpx.Op("projects-get", "GET", httpx.V1+"/projects/{id}", "Get project", "Projects", mw...), h.get)
	huma.Register(api, httpx.Op("projects-update", "PUT", httpx.V1+"/projects/{id}", "Update project", "Projects", mw...), h.update)
	huma.Register(api, httpx.Op("projects-delete", "DELETE", httpx.V1+"/projects/{id}", "Delete project", "Projects", mw...), h.delete)
	huma.Register(api, httpx.Op("projects-containers", "GET", httpx.V1+"/projects/{id}/containers", "Project containers", "Projects", mw...), h.listContainers)
}
