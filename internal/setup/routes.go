package setup

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API) {
	huma.Register(api, httpx.Op(
		"setup-admin", "POST", httpx.V1+"/setup/admin",
		"Create initial admin user (only works if no users exist)", "Setup",
	), h.setupAdmin)
}
