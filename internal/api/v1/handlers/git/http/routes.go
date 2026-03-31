package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("git-list", "GET", "/api/v1/git/integrations", "List integrations", "Git", mw...),
		h.List,
	)
	huma.Register(
		api,
		shared.Op("git-create", "POST", "/api/v1/git/integrations", "Create integration", "Git", mw...),
		h.Create,
	)
	huma.Register(
		api,
		shared.Op("git-get", "GET", "/api/v1/git/integrations/{id}", "Get integration", "Git", mw...),
		h.Get,
	)
	huma.Register(
		api,
		shared.Op("git-delete", "DELETE", "/api/v1/git/integrations/{id}", "Delete integration", "Git", mw...),
		h.Delete,
	)
	huma.Register(
		api,
		shared.Op("git-validate", "POST", "/api/v1/git/integrations/{id}/validate", "Validate token", "Git", mw...),
		h.Validate,
	)
	huma.Register(
		api,
		shared.Op("git-repos", "GET", "/api/v1/git/integrations/{id}/repositories", "List repositories", "Git", mw...),
		h.ListRepositories,
	)
	huma.Register(
		api,
		shared.Op("git-shares", "PUT", "/api/v1/git/integrations/{id}/shares", "Set shares", "Git", mw...),
		h.SetShares,
	)
	huma.Register(
		api,
		shared.Op(
			"git-branches",
			"GET",
			"/api/v1/git/integrations/{id}/repositories/{owner}/{repo}/branches",
			"List branches",
			"Git",
			mw...,
		),
		h.ListBranches,
	)
}
