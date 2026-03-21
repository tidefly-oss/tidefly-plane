package http

import (
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
)

const refreshTokenCookie = "tfy_rt"

func isDevMode() bool {
	return os.Getenv("APP_ENV") == "development"
}

func setRefreshCookie(ctx huma.Context, token string) {
	cookie := &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   !isDevMode(),         // false in dev (HTTP), true in prod (HTTPS)
		SameSite: http.SameSiteLaxMode, // Lax erlaubt cross-origin bei top-level navigation
	}
	ctx.AppendHeader("Set-Cookie", cookie.String())
}

func clearRefreshCookie(ctx huma.Context) {
	cookie := &http.Cookie{
		Name:     refreshTokenCookie,
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !isDevMode(),
		SameSite: http.SameSiteLaxMode,
	}
	ctx.AppendHeader("Set-Cookie", cookie.String())
}

func getRefreshCookie(ctx huma.Context) string {
	c, _ := huma.ReadCookie(ctx, refreshTokenCookie)
	if c == nil {
		return ""
	}
	return c.Value
}
