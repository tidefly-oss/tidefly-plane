package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	huma.Register(api, shared.Op("images-list", "GET", "/api/v1/images", "List images", mw...), h.List)
	huma.Register(
		api,
		shared.Op("images-containers", "GET", "/api/v1/images/{id}/containers", "Containers using image", mw...),
		h.Containers,
	)
	huma.Register(
		api,
		shared.Op("images-delete", "DELETE", "/api/v1/images/{id}", "Delete image", adminMw...),
		h.Delete,
	)
}
