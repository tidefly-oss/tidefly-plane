package middleware

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

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
	if err := access.CheckProjectMembership(db, claims.UserID, labels); err != nil {
		return huma.Error403Forbidden(err.Error())
	}
	return nil
}
