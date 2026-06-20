package volume

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	huma.Register(api, httpx.Op("volumes-list", "GET", httpx.V1+"/volumes", "List volumes", "Volumes", mw...), h.list)
	huma.Register(api, httpx.Op("volumes-containers", "GET", httpx.V1+"/volumes/{id}/containers", "Containers using volume", "Volumes", mw...), h.containers)
	huma.Register(api, httpx.Op("volumes-delete", "DELETE", httpx.V1+"/volumes/{id}", "Delete volume", "Volumes", adminMw...), h.delete)
}
