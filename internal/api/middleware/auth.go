// Package middleware provides HTTP and Huma middleware for authentication, authorization, and request enrichment.
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
type humaIPKey struct{}
type humaUserAgentKey struct{}

// ── Huma Middleware ───────────────────────────────────────────────────────────

// RequireAuthHuma validates JWT from Authorization header for Huma routes.
// Also injects IP and User-Agent into the context for audit logging.
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

		// Resolve real IP — prefer X-Real-IP set by Caddy, fall back to RemoteAddr
		ip := ctx.Header("X-Real-IP")
		if ip == "" {
			ip = ctx.Header("X-Forwarded-For")
			if idx := strings.IndexByte(ip, ','); idx != -1 {
				ip = strings.TrimSpace(ip[:idx])
			}
		}
		if ip == "" {
			ip = ctx.RemoteAddr()
		}

		ua := ctx.Header("User-Agent")

		newCtx := context.WithValue(ctx.Context(), humaUserKey{}, claims)
		newCtx = context.WithValue(newCtx, humaIPKey{}, ip)
		newCtx = context.WithValue(newCtx, humaUserAgentKey{}, ua)
		next(huma.WithContext(ctx, newCtx))
	}
}

// UserFromHumaCtx returns *auth.Claims from a Huma/stdlib context.
func UserFromHumaCtx(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(humaUserKey{}).(*auth.Claims)
	return claims
}

// IPFromCtx returns the client IP injected by RequireAuthHuma.
func IPFromCtx(ctx context.Context) string {
	ip, _ := ctx.Value(humaIPKey{}).(string)
	return ip
}

// UserAgentFromCtx returns the User-Agent injected by RequireAuthHuma.
func UserAgentFromCtx(ctx context.Context) string {
	ua, _ := ctx.Value(humaUserAgentKey{}).(string)
	return ua
}

// InjectHumaContext stores the huma.Context inside the stdlib context.
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

func extractBearerToken(header string) string {
	if strings.HasPrefix(header, "Bearer ") {
		return header[7:]
	}
	return ""
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

// ── ContextEnricher implementation ───────────────────────────────────────────

// Enricher implements logger.ContextEnricher using the injected context keys.
type Enricher struct{}

func NewEnricher() *Enricher { return &Enricher{} }

func (e *Enricher) IP(ctx context.Context) string        { return IPFromCtx(ctx) }
func (e *Enricher) UserAgent(ctx context.Context) string { return UserAgentFromCtx(ctx) }
func (e *Enricher) UserEmail(ctx context.Context) string {
	if claims := UserFromHumaCtx(ctx); claims != nil {
		return claims.Email
	}
	return ""
}
