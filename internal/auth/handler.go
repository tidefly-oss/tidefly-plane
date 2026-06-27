package auth

import (
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc *AuthService
	log *logger.Logger
}

// NewHandler wires db + jwt + tokenStore into AuthService and returns a Handler.
// Signature matches bootstrap/register.go: NewHandler(db, jwtSvc, tokenStore, log)
func NewHandler(db *gorm.DB, jwtSvc *JWTService, tokenStore *TokenStore, log *logger.Logger) *Handler {
	store := NewStore(db)
	svc := NewAuthService(store, jwtSvc, tokenStore)
	return &Handler{svc: svc, log: log}
}

// ── Token response ────────────────────────────────────────────────────────────

type tokenOutput struct {
	Body struct {
		AccessToken string    `json:"access_token"`
		TokenType   string    `json:"token_type"`
		ExpiresIn   int       `json:"expires_in"`
		ExpiresAt   time.Time `json:"expires_at"`
	}
}

func newTokenOutput(accessToken string) *tokenOutput {
	out := &tokenOutput{}
	out.Body.AccessToken = accessToken
	out.Body.TokenType = "Bearer"
	out.Body.ExpiresIn = 15 * 60
	out.Body.ExpiresAt = time.Now().Add(15 * time.Minute)
	return out
}

// ── Cookie helpers ────────────────────────────────────────────────────────────

const refreshCookieName = "tfy_rt"

func setRefreshCookie(ctx huma.Context, token string) {
	ctx.AppendHeader("Set-Cookie", (&http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   !isDev(),
		SameSite: http.SameSiteLaxMode,
	}).String())
}

func clearRefreshCookie(ctx huma.Context) {
	ctx.AppendHeader("Set-Cookie", (&http.Cookie{
		Name:     refreshCookieName,
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !isDev(),
		SameSite: http.SameSiteLaxMode,
	}).String())
}

func getRefreshCookie(ctx huma.Context) string {
	c, _ := huma.ReadCookie(ctx, refreshCookieName)
	if c == nil {
		return ""
	}
	return c.Value
}

func isDev() bool { return os.Getenv("APP_ENV") == "development" }
