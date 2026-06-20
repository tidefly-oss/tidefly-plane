package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// SessionUser holds the claims extracted from a validated JWT.
// Defined here so middleware has no dependency on the auth package.
type SessionUser struct {
	UserID string
	Email  string
	Role   string
}

// ── Context keys ──────────────────────────────────────────────────────────────

type humaUserKey struct{}
type humaCtxKey struct{}
type humaIPKey struct{}
type humaUserAgentKey struct{}

// ── Huma middleware ───────────────────────────────────────────────────────────

// RequireAuthHuma validates the JWT and injects the session user + IP + UA into context.
func RequireAuthHuma(api huma.API, validate func(string) (*SessionUser, error)) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token := extractBearer(ctx.Header("Authorization"))
		if token == "" {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "missing authorization header", nil)
			return
		}
		// Sanity-check token length to reject obviously malformed inputs early
		if len(token) > 2048 {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid token", nil)
			return
		}
		user, err := validate(token)
		if err != nil {
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "invalid or expired token", nil)
			return
		}

		ip := realIP(ctx.Header("X-Real-IP"), ctx.Header("X-Forwarded-For"), ctx.RemoteAddr())

		newCtx := context.WithValue(ctx.Context(), humaUserKey{}, user)
		newCtx = context.WithValue(newCtx, humaIPKey{}, ip)
		newCtx = context.WithValue(newCtx, humaUserAgentKey{}, ctx.Header("User-Agent"))
		next(huma.WithContext(ctx, newCtx))
	}
}

// RequireAuthSSE validates JWT from Authorization header or ?token= query param.
// Used for SSE and WebSocket endpoints where custom headers are not available.
func RequireAuthSSE(validate func(string) (*SessionUser, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r.Header.Get("Authorization"))
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token == "" || len(token) > 2048 {
				http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			user, err := validate(token)
			if err != nil {
				http.Error(w, `{"message":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), humaUserKey{}, user)
			ctx = context.WithValue(ctx, humaIPKey{}, realIP(
				r.Header.Get("X-Real-IP"),
				r.Header.Get("X-Forwarded-For"),
				r.RemoteAddr,
			))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ── Context accessors ─────────────────────────────────────────────────────────

func UserFromHumaCtx(ctx context.Context) *SessionUser {
	u, _ := ctx.Value(humaUserKey{}).(*SessionUser)
	return u
}

func IPFromCtx(ctx context.Context) string {
	ip, _ := ctx.Value(humaIPKey{}).(string)
	return ip
}

func UserAgentFromCtx(ctx context.Context) string {
	ua, _ := ctx.Value(humaUserAgentKey{}).(string)
	return ua
}

// ── Huma context ──────────────────────────────────────────────────────────────

func InjectHumaContext() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		newCtx := context.WithValue(ctx.Context(), humaCtxKey{}, ctx)
		next(huma.WithContext(ctx, newCtx))
	}
}

func HumaContextFrom(ctx context.Context) huma.Context {
	hc, _ := ctx.Value(humaCtxKey{}).(huma.Context)
	return hc
}

// ── Logger enricher ───────────────────────────────────────────────────────────

type Enricher struct{}

func NewEnricher() *Enricher { return &Enricher{} }

func (e *Enricher) IP(ctx context.Context) string        { return IPFromCtx(ctx) }
func (e *Enricher) UserAgent(ctx context.Context) string { return UserAgentFromCtx(ctx) }
func (e *Enricher) UserEmail(ctx context.Context) string {
	if u := UserFromHumaCtx(ctx); u != nil {
		return u.Email
	}
	return ""
}

// ── helpers ───────────────────────────────────────────────────────────────────

func extractBearer(header string) string {
	if strings.HasPrefix(header, "Bearer ") {
		t := header[7:]
		// Strip any accidental whitespace
		return strings.TrimSpace(t)
	}
	return ""
}

// realIP extracts the best available client IP, preferring trusted proxy headers.
func realIP(xRealIP, xForwardedFor, remoteAddr string) string {
	if xRealIP != "" {
		return strings.TrimSpace(xRealIP)
	}
	if xForwardedFor != "" {
		// Take the first (leftmost) IP — the original client
		if idx := strings.IndexByte(xForwardedFor, ','); idx != -1 {
			return strings.TrimSpace(xForwardedFor[:idx])
		}
		return strings.TrimSpace(xForwardedFor)
	}
	// Strip port from RemoteAddr
	if idx := strings.LastIndexByte(remoteAddr, ':'); idx != -1 {
		return remoteAddr[:idx]
	}
	return remoteAddr
}
