package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tidefly-oss/tidefly-plane/internal/domain/auth"
)

// ── Context keys ──────────────────────────────────────────────────────────────

type humaUserKey struct{}
type humaCtxKey struct{}

// ── Huma Middleware ───────────────────────────────────────────────────────────

// RequireAuthHuma validates JWT from Authorization header for Huma routes.
func RequireAuthHuma(api huma.API, jwtSvc *auth.Service) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token := extractBearerToken(ctx.Header("Authorization"))
		if token == "" {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing authorization header", nil)
			return
		}
		claims, err := jwtSvc.ValidateAccessToken(token)
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid or expired token", nil)
			return
		}
		newCtx := context.WithValue(ctx.Context(), humaUserKey{}, claims)
		next(huma.WithContext(ctx, newCtx))
	}
}

// UserFromHumaCtx returns *auth.Claims from a Huma/stdlib context.
func UserFromHumaCtx(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(humaUserKey{}).(*auth.Claims)
	return claims
}

// InjectHumaContext stores the huma.Context inside the stdlib context
// so service-layer code can retrieve it if needed.
func InjectHumaContext() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		newCtx := context.WithValue(ctx.Context(), humaCtxKey{}, ctx)
		next(huma.WithContext(ctx, newCtx))
	}
}

// HumaContextFrom retrieves a huma.Context from a stdlib context.Context.
func HumaContextFrom(ctx context.Context) huma.Context {
	hc, _ := ctx.Value(humaCtxKey{}).(huma.Context)
	return hc
}

// ── SSE / WebSocket Middleware (stdlib) ───────────────────────────────────────

// RequireAuthSSE validates JWT from Authorization header OR ?token= query param.
// EventSource and WebSocket clients cannot set custom headers — token goes in query.
func RequireAuthSSE(jwtSvc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r.Header.Get("Authorization"))
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			claims, err := jwtSvc.ValidateAccessToken(token)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), humaUserKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ── internal ──────────────────────────────────────────────────────────────────

func extractBearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
