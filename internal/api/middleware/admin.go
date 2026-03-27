package middleware

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// ── Echo helpers ──────────────────────────────────────────────────────────────

func InjectUser(db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			token, ok := c.Get("user").(*auth.Claims)
			if !ok || token == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
			}

			var dbUser models.User
			if err := db.First(&dbUser, "id = ?", token.UserID).Error; err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "user not found"})
			}

			if !dbUser.Active {
				return c.JSON(http.StatusForbidden, map[string]string{"message": "account inactive"})
			}

			c.Set("user", &dbUser)
			return next(c)
		}
	}
}

func UserFromContext(c *echo.Context) *models.User {
	u, _ := c.Get("user").(*models.User)
	return u
}

// ── Huma helpers ──────────────────────────────────────────────────────────────

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
