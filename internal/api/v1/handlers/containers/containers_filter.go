package containers

// ── Project-based filtering for Members ──────────────────────────────────────
//
// How it works:
//
//  1. Admin → sees all containers, no filtering.
//
//  2. Member → the Docker runtime returns all containers, but we filter
//     in-memory by project network membership.
//     A container "belongs" to a project if it is connected to that project's
//     network (Project.NetworkName, e.g. "tidefly_myproject").
//
//  3. The allowed network names are derived from the user's ProjectMembers
//     by joining with the projects table once per request.
//
// Usage inside any handler method:
//
//	user := middleware.UserFromContext(c)
//	if user != nil && !user.IsAdmin() {
//	    nets, err := h.allowedNetworks(user.ID)
//	    if err != nil { return echo.ErrInternalServerError }
//	    containers = filterByNetworks(containers, nets)
//	}

import (
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

// allowedNetworks returns the set of Docker network names the user may access.
// For admins this is never called. For members it queries their project memberships.
func (h *Handler) allowedNetworks(userID string) (map[string]struct{}, error) {
	var members []models.ProjectMember
	if err := h.db.Where("user_id = ?", userID).Find(&members).Error; err != nil {
		return nil, err
	}

	if len(members) == 0 {
		return map[string]struct{}{}, nil
	}

	projectIDs := make([]string, len(members))
	for i, m := range members {
		projectIDs[i] = m.ProjectID
	}

	var projects []models.Project
	if err := h.db.Select("network_name").Where("id IN ?", projectIDs).Find(&projects).Error; err != nil {
		return nil, err
	}

	nets := make(map[string]struct{}, len(projects))
	for _, p := range projects {
		if p.NetworkName != "" {
			nets[p.NetworkName] = struct{}{}
		}
	}
	return nets, nil
}

// containerAllowed returns true if the container is connected to at least one
// of the allowed networks. Uses the Networks field ([]string) on the container.
func containerAllowed(networks []string, allowed map[string]struct{}) bool {
	for _, n := range networks {
		if _, ok := allowed[n]; ok {
			return true
		}
	}
	return false
}
