package admin

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/httpx"
)

func (h *Handler) RegisterRoutes(api huma.API, adminMw huma.Middlewares) {
	// ── Users ─────────────────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("admin-users-list", "GET", httpx.V1+"/admin/users", "List users", "Admin", adminMw...), h.listUsers)
	huma.Register(api, httpx.Op("admin-users-create", "POST", httpx.V1+"/admin/users", "Create user", "Admin", adminMw...), h.createUser)
	huma.Register(api, httpx.Op("admin-users-get", "GET", httpx.V1+"/admin/users/{id}", "Get user", "Admin", adminMw...), h.getUser)
	huma.Register(api, httpx.Op("admin-users-update", "PATCH", httpx.V1+"/admin/users/{id}", "Update user", "Admin", adminMw...), h.updateUser)
	huma.Register(api, httpx.Op("admin-users-delete", "DELETE", httpx.V1+"/admin/users/{id}", "Delete user", "Admin", adminMw...), h.deleteUser)
	huma.Register(api, httpx.Op("admin-users-reset-pw", "POST", httpx.V1+"/admin/users/{id}/reset-password", "Reset password", "Admin", adminMw...), h.resetUserPassword)
	huma.Register(api, httpx.Op("admin-users-projects", "PUT", httpx.V1+"/admin/users/{id}/projects", "Set project members", "Admin", adminMw...), h.setProjectMembers)

	// ── Settings ──────────────────────────────────────────────────────────────
	huma.Register(api, httpx.Op("admin-settings-get", "GET", httpx.V1+"/admin/settings", "Get settings", "Admin", adminMw...), h.getSettings)
	huma.Register(api, httpx.Op("admin-settings-update", "PATCH", httpx.V1+"/admin/settings", "Update settings", "Admin", adminMw...), h.updateSettings)
	huma.Register(api, httpx.Op("admin-settings-test", "POST", httpx.V1+"/admin/settings/test/{channel}", "Test notification channel", "Admin", adminMw...), h.testNotification)
}
