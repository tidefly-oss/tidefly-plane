package dashboard

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(
		api,
		httpx.Op("dashboard-overview", "GET", httpx.V1+"/dashboard/overview", "Dashboard overview", "Dashboard", mw...),
		h.overview,
	)
}
