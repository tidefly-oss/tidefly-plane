package template

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(api, httpx.Op("templates-list", "GET", httpx.V1+"/services/templates", "List templates", "Templates", mw...), h.listTemplates)
	huma.Register(api, httpx.Op("templates-get", "GET", httpx.V1+"/services/templates/{slug}", "Get template", "Templates", mw...), h.getTemplate)
}
