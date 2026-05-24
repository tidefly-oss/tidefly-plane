package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// ── Role Middleware ───────────────────────────────────────────────────────────

// RequireAdminHuma checks admin role from JWT claims — no DB lookup needed.
func RequireAdminHuma(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		claims := UserFromHumaCtx(ctx.Context())
		if claims == nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "unauthorized", nil)
			return
		}
		if claims.Role != string(models.RoleAdmin) {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "admin access required", nil)
			return
		}
		next(ctx)
	}
}

// ── Container Access ──────────────────────────────────────────────────────────

// CheckContainerAccess checks if the authenticated user may access a container.
// Admins always pass. Members must be part of the container's project.
func CheckContainerAccess(ctx context.Context, db *gorm.DB, labels map[string]string) error {
	claims := UserFromHumaCtx(ctx)
	if claims == nil {
		return huma.Error401Unauthorized("unauthorized")
	}
	if claims.Role == string(models.RoleAdmin) {
		return nil
	}
	if err := checkProjectMembership(db, claims.UserID, labels); err != nil {
		return huma.Error403Forbidden(err.Error())
	}
	return nil
}

func checkProjectMembership(db *gorm.DB, userID string, labels map[string]string) error {
	projectID, exists := labels["tidefly-plane.project_id"]
	if !exists || projectID == "" {
		return fmt.Errorf("access denied: container is not part of any project")
	}
	var count int64
	if err := db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("access check failed")
	}
	if count == 0 {
		return fmt.Errorf("access denied: you are not a member of this container's project")
	}
	return nil
}
