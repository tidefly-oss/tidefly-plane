package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API) {
	huma.Register(api, shared.Op(
		"setup-admin", "POST", "/api/v1/setup/admin",
		"Create initial admin user (only works if no users exist)", "Setup",
	), h.SetupAdmin)
}
