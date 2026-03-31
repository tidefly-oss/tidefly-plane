package http

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/shared"
)

func (h *Handler) RegisterRoutes(api huma.API, _ huma.Middlewares, adminMw huma.Middlewares) {
	huma.Register(
		api,
		shared.Op("admin-users-list", "GET", "/api/v1/admin/users", "List users", "Admin", adminMw...),
		h.ListUsers,
	)
	huma.Register(
		api,
		shared.Op("admin-users-create", "POST", "/api/v1/admin/users", "Create user", "Admin", adminMw...),
		h.CreateUser,
	)
	huma.Register(
		api,
		shared.Op("admin-users-get", "GET", "/api/v1/admin/users/{id}", "Get user", "Admin", adminMw...),
		h.GetUser,
	)
	huma.Register(
		api,
		shared.Op("admin-users-update", "PATCH", "/api/v1/admin/users/{id}", "Update user", "Admin", adminMw...),
		h.UpdateUser,
	)
	huma.Register(
		api,
		shared.Op("admin-users-delete", "DELETE", "/api/v1/admin/users/{id}", "Delete user", "Admin", adminMw...),
		h.DeleteUser,
	)
	huma.Register(
		api,
		shared.Op(
			"admin-users-reset-pw",
			"POST",
			"/api/v1/admin/users/{id}/reset-password",
			"Reset password",
			"Admin",
			adminMw...,
		),
		h.ResetUserPassword,
	)
	huma.Register(
		api,
		shared.Op(
			"admin-users-projects",
			"PUT",
			"/api/v1/admin/users/{id}/projects",
			"Set project members",
			"Admin",
			adminMw...,
		),
		h.SetProjectMembers,
	)
	huma.Register(
		api,
		shared.Op("admin-settings-get", "GET", "/api/v1/admin/settings", "Get settings", "Admin", adminMw...),
		h.GetSettings,
	)
	huma.Register(
		api,
		shared.Op("admin-settings-update", "PATCH", "/api/v1/admin/settings", "Update settings", "Admin", adminMw...),
		h.UpdateSettings,
	)
	huma.Register(
		api,
		shared.Op(
			"admin-settings-test",
			"POST",
			"/api/v1/admin/settings/test/{channel}",
			"Test notification channel",
			"Admin",
			adminMw...,
		),
		h.TestNotification,
	)
}
