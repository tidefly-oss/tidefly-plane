package middleware

import (
	"net/http"

	"github.com/aarondl/authboss/v3"
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

// ── Echo Middleware (unverändert) ─────────────────────────────────────────────

func InjectUser(ab *authboss.Authboss, db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			w := ab.NewResponse(c.Response())
			r, err := ab.LoadClientState(w, c.Request())
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
			}
			userIface, err := ab.LoadCurrentUser(&r)
			if err != nil || userIface == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
			}
			u, ok := userIface.(*models.User)
			if !ok {
				return c.JSON(http.StatusInternalServerError, map[string]string{"message": "user type error"})
			}
			var dbUser models.User
			if err := db.First(&dbUser, "id = ?", u.ID).Error; err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "user not found"})
			}
			c.Set("user", &dbUser)
			return next(c)
		}
	}
}

func RequireAdmin(ab *authboss.Authboss, db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			w := ab.NewResponse(c.Response())
			r, err := ab.LoadClientState(w, c.Request())
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
			}
			userIface, err := ab.LoadCurrentUser(&r)
			if err != nil || userIface == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
			}
			u, ok := userIface.(*models.User)
			if !ok {
				return c.JSON(http.StatusInternalServerError, map[string]string{"message": "user type error"})
			}
			var dbUser models.User
			if err := db.First(&dbUser, "id = ?", u.ID).Error; err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
			}
			if !dbUser.IsAdmin() {
				return c.JSON(http.StatusForbidden, map[string]string{"message": "admin access required"})
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

// ── Huma Middleware ───────────────────────────────────────────────────────────
// api wird per Closure reingezogen — huma.Context hat kein API()-Method.

func RequireAdminHuma(api huma.API, db *gorm.DB) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		abUser := UserFromHumaCtx(ctx.Context())
		if abUser == nil {
			err := huma.WriteErr(api, ctx, http.StatusUnauthorized, "unauthorized", nil)
			if err != nil {
				return
			}
			return
		}
		u, ok := abUser.(*models.User)
		if !ok {
			err := huma.WriteErr(api, ctx, http.StatusInternalServerError, "user type error", nil)
			if err != nil {
				return
			}
			return
		}
		var dbUser models.User
		if err := db.First(&dbUser, "id = ?", u.ID).Error; err != nil {
			err := huma.WriteErr(api, ctx, http.StatusUnauthorized, "user not found", nil)
			if err != nil {
				return
			}
			return
		}
		if !dbUser.IsAdmin() {
			err := huma.WriteErr(api, ctx, http.StatusForbidden, "admin access required", nil)
			if err != nil {
				return
			}
			return
		}
		next(ctx)
	}
}

func UserFromHumaCtxTyped(ctx huma.Context) *models.User {
	abUser := UserFromHumaCtx(ctx.Context())
	if abUser == nil {
		return nil
	}
	u, _ := abUser.(*models.User)
	return u
}
