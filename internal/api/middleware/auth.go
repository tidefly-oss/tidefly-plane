package middleware

import (
	"context"
	"net/http"

	"github.com/aarondl/authboss/v3"
	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
)

// ── Echo Middleware (unverändert) ─────────────────────────────────────────────

func RequireAuth(ab *authboss.Authboss) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			w := ab.NewResponse(c.Response())
			r, err := ab.LoadClientState(w, c.Request())
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			if _, err := ab.LoadCurrentUser(&r); err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			return next(c)
		}
	}
}

// ── Huma Middleware ───────────────────────────────────────────────────────────
// Signatur: func(huma.Context, func(huma.Context))
// huma.Context hat kein API()-Method — die huma.API Instanz wird per Closure
// reingezogen, nicht vom Context gelesen.

func RequireAuthHuma(api huma.API, ab *authboss.Authboss) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		cookie := ctx.Header("Cookie")

		r, _ := http.NewRequestWithContext(ctx.Context(), http.MethodGet, "/", nil)
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}

		w := ab.NewResponse(&discardResponseWriter{})
		nr, err := ab.LoadClientState(w, r)

		if err != nil {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "unauthorized", nil)
			return
		}
		user, err := ab.LoadCurrentUser(&nr)

		if err != nil || user == nil {
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "unauthorized", nil)
			return
		}

		newCtx := context.WithValue(ctx.Context(), humaUserKey{}, user)
		next(huma.WithContext(ctx, newCtx))
	}
}

// ── Context helpers ───────────────────────────────────────────────────────────

type humaUserKey struct{}

func UserFromHumaCtx(ctx context.Context) authboss.User {
	u, _ := ctx.Value(humaUserKey{}).(authboss.User)
	return u
}

// discardResponseWriter fängt Authboss-Writes ab die wir nicht brauchen.
type discardResponseWriter struct {
	header http.Header
}

func (d *discardResponseWriter) Header() http.Header {
	if d.header == nil {
		d.header = make(http.Header)
	}
	return d.header
}
func (d *discardResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (d *discardResponseWriter) WriteHeader(_ int)           {}
