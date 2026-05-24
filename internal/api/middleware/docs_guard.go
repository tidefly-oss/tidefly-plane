package middleware

import (
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// GuardDocs blocks /docs and /openapi routes when api_docs_enabled is false.
// Checked live on every request — no restart needed.
func GuardDocs(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if !strings.HasPrefix(path, "/docs") && !strings.HasPrefix(path, "/openapi") {
				next.ServeHTTP(w, r)
				return
			}
			var s models.SystemSettings
			if err := db.First(&s).Error; err == nil && !s.APIDocsEnabled {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
