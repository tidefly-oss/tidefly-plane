// Package http provides the HTTP handler for the dashboard overview aggregation endpoint.
package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, mw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("dashboard-overview", "GET", "/api/v1/dashboard/overview", "Dashboard overview", "Dashboard", mw...),
		h.Overview,
	)
}
