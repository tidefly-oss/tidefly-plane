package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("networks-list", "GET", "/api/v1/networks", "List networks", "Networks", mw...),
		h.List,
	)
	huma.Register(
		api,
		shared.Op("networks-get", "GET", "/api/v1/networks/{id}", "Get network", "Networks", mw...),
		h.Get,
	)
	huma.Register(
		api,
		shared.Op(
			"networks-containers",
			"GET",
			"/api/v1/networks/{id}/containers",
			"Network containers",
			"Networks",
			mw...,
		),
		h.Containers,
	)
	huma.Register(
		api,
		shared.Op("networks-delete", "DELETE", "/api/v1/networks/{id}", "Delete network", "Networks", adminMw...),
		h.Delete,
	)
}
