package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
)

// CORS configures allowed origins from CORS_ORIGINS env var.
// Falls back to localhost:5173 for local development.
func CORS() echo.MiddlewareFunc {
	origins := allowedOrigins()

	return echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{echo.HeaderContentType, echo.HeaderAuthorization, "Cookie"},
		AllowCredentials: true,
	})
}

func allowedOrigins() []string {
	if env := os.Getenv("CORS_ORIGINS"); env != "" {
		return strings.Split(env, ",")
	}
	return []string{"http://localhost:5173", "http://localhost:5174"}
}
