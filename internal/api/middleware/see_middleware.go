package middleware

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/auth"
)

// RequireAuthSSE validates JWT from Authorization header OR ?token= query param.
// EventSource and WebSocket cannot set custom headers — token must be in query.
func RequireAuthSSE(jwtSvc *auth.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			// 1. Try Authorization header first
			token := extractBearerToken(c.Request().Header.Get("Authorization"))

			// 2. Fall back to ?token= query param (EventSource / WebSocket)
			if token == "" {
				token = c.QueryParam("token")
			}

			if token == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or missing token")
			}

			claims, err := jwtSvc.ValidateAccessToken(token)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or missing token")
			}

			// Store claims same as RequireAuth so InjectUser still works
			c.Set("user", claims)
			return next(c)
		}
	}
}
