package image

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares, adminMw huma.Middlewares) {
	huma.Register(api, httpx.Op("images-list", "GET", httpx.V1+"/images", "List images", "Images", mw...), h.list)
	huma.Register(api, httpx.Op("images-containers", "GET", httpx.V1+"/images/{id}/containers", "Containers using image", "Images", mw...), h.containers)
	huma.Register(api, httpx.Op("images-delete", "DELETE", httpx.V1+"/images/{id}", "Delete image", "Images", adminMw...), h.delete)
}
