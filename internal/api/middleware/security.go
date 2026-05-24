package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/cors"
)

// ── CORS ──────────────────────────────────────────────────────────────────────

// CORS configures allowed origins from CORS_ORIGINS env var.
// Falls back to localhost:5173/5174 for local development.
func CORS() func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins(),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "Cookie"},
		AllowCredentials: true,
	})
}

func allowedOrigins() []string {
	if env := os.Getenv("CORS_ORIGINS"); env != "" {
		return strings.Split(env, ",")
	}
	return []string{"http://localhost:5173", "http://localhost:5174"}
}

// ── Security Headers ──────────────────────────────────────────────────────────

// SecurityHeaders sets HTTP security headers equivalent to helmet.js.
// Apply globally before all routes.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-XSS-Protection", "1; mode=block")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			// Uncomment when TLS is terminated at app level:
			// h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

			if strings.HasPrefix(r.URL.Path, "/swagger") {
				h.Set("Content-Security-Policy",
					"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
			} else {
				h.Set("Content-Security-Policy", "default-src 'none'")
			}
			h.Del("Server")

			next.ServeHTTP(w, r)
		})
	}
}
