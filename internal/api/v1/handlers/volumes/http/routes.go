package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	huma.Register(api, shared.Op("volumes-list", "GET", "/api/v1/volumes", "List volumes", "Volumes", mw...), h.List)
	huma.Register(
		api,
		shared.Op(
			"volumes-containers",
			"GET",
			"/api/v1/volumes/{id}/containers",
			"Containers using volume",
			"Volumes",
			mw...,
		),
		h.Containers,
	)
	huma.Register(
		api,
		shared.Op("volumes-delete", "DELETE", "/api/v1/volumes/{id}", "Delete volume", "Volumes", adminMw...),
		h.Delete,
	)
}
