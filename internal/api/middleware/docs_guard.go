package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

// GuardDocs blocks /docs and /openapi routes when api_docs_enabled is false.
// Checked live on every request — no restart needed.
func GuardDocs(db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			path := (*c).Request().URL.Path
			if !strings.HasPrefix(path, "/docs") && !strings.HasPrefix(path, "/openapi") {
				return next(c)
			}
			var s models.SystemSettings
			if err := db.First(&s).Error; err == nil && !s.APIDocsEnabled {
				return (*c).NoContent(http.StatusNotFound)
			}
			return next(c)
		}
	}
}
