package middleware

import (
	"strings"

	"github.com/labstack/echo/v5"
)

// SecurityHeaders sets HTTP security headers equivalent to helmet.js.
// Should be applied globally before all routes.
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			h := c.Response().Header()

			// Prevent MIME type sniffing
			h.Set("X-Content-Type-Options", "nosniff")
			// Prevent clickjacking
			h.Set("X-Frame-Options", "DENY")
			// XSS protection (legacy browsers)
			h.Set("X-XSS-Protection", "1; mode=block")
			// Force HTTPS (only set in production — dev uses HTTP)
			// Uncomment when TLS is terminated at the app level:
			// h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			// Referrer policy
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			// Permissions policy — disable browser features we don't need
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			// Content Security Policy
			// Swagger UI needs scripts, styles and images — relax CSP for that route only.
			if strings.HasPrefix(c.Request().URL.Path, "/swagger") {
				h.Set(
					"Content-Security-Policy",
					"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:",
				)
			} else {
				// Tidefly is an API — no HTML served, so we lock it down hard.
				h.Set("Content-Security-Policy", "default-src 'none'")
			}
			// Remove server fingerprint
			h.Del("Server")

			return next(c)
		}
	}
}
