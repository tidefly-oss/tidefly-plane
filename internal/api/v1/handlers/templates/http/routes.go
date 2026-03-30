package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("templates-list", "GET", "/api/v1/services/templates", "List templates", "Templates", mw...),
		h.ListTemplates,
	)
	huma.Register(
		api,
		shared.Op("templates-get", "GET", "/api/v1/services/templates/{slug}", "Get template", "Templates", mw...),
		h.GetTemplate,
	)
}
