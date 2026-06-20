package git

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, httpx.Op("git-list", "GET", httpx.V1+"/git/integrations", "List integrations", "Git", mw...), h.list)
	huma.Register(api, httpx.Op("git-create", "POST", httpx.V1+"/git/integrations", "Create integration", "Git", mw...), h.create)
	huma.Register(api, httpx.Op("git-get", "GET", httpx.V1+"/git/integrations/{id}", "Get integration", "Git", mw...), h.get)
	huma.Register(api, httpx.Op("git-delete", "DELETE", httpx.V1+"/git/integrations/{id}", "Delete integration", "Git", mw...), h.delete)
	huma.Register(api, httpx.Op("git-validate", "POST", httpx.V1+"/git/integrations/{id}/validate", "Validate token", "Git", mw...), h.validate)
	huma.Register(api, httpx.Op("git-repos", "GET", httpx.V1+"/git/integrations/{id}/repositories", "List repositories", "Git", mw...), h.listRepositories)
	huma.Register(api, httpx.Op("git-shares", "PUT", httpx.V1+"/git/integrations/{id}/shares", "Set shares", "Git", mw...), h.setShares)
	huma.Register(api, httpx.Op("git-branches", "GET", httpx.V1+"/git/integrations/{id}/repositories/{owner}/{repo}/branches", "List branches", "Git", mw...), h.listBranches)
}
