package network

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	huma.Register(api, httpx.Op("networks-list", "GET", httpx.V1+"/networks", "List networks", "Networks", mw...), h.list)
	huma.Register(api, httpx.Op("networks-get", "GET", httpx.V1+"/networks/{id}", "Get network", "Networks", mw...), h.get)
	huma.Register(api, httpx.Op("networks-containers", "GET", httpx.V1+"/networks/{id}/containers", "Network containers", "Networks", mw...), h.containers)
	huma.Register(api, httpx.Op("networks-delete", "DELETE", httpx.V1+"/networks/{id}", "Delete network", "Networks", adminMw...), h.delete)
}
