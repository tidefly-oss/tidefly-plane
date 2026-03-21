package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-backend/internal/auth"
)

// ── Context keys ──────────────────────────────────────────────────────────────

type humaUserKey struct{}

type humaCtxKey struct{}

// ── Huma Middleware ───────────────────────────────────────────────────────────

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

// UserFromHumaCtx returns *auth.Claims from a Huma context.
func UserFromHumaCtx(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(humaUserKey{}).(*auth.Claims)
	return claims
}

// ── Echo Middleware ───────────────────────────────────────────────────────────

func InjectHumaContext() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		newCtx := context.WithValue(ctx.Context(), humaCtxKey{}, ctx)
		next(huma.WithContext(ctx, newCtx))
	}
}

// HumaContextFrom extrahiert den huma.Context aus einem stdlib context.Context.
func HumaContextFrom(ctx context.Context) huma.Context {
	hc, _ := ctx.Value(humaCtxKey{}).(huma.Context)
	return hc
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
