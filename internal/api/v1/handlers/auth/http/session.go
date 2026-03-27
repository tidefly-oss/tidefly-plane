package http

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/auth/service"
)

const tokenTypeBearer = "Bearer"

// ── Token response ────────────────────────────────────────────────────────────
// Refresh token is NOT in the JSON body — it lives in an HttpOnly cookie.
// Only the access token is returned to JS.

type TokenOutput struct {
	Body struct {
		AccessToken string    `json:"access_token"`
		TokenType   string    `json:"token_type"`
		ExpiresIn   int       `json:"expires_in"`
		ExpiresAt   time.Time `json:"expires_at"`
	}
}

// ── Register ──────────────────────────────────────────────────────────────────

type RegisterInput struct {
	Body struct {
		Name     string `json:"name"     minLength:"1"  maxLength:"255"`
		Email    string `json:"email"    format:"email"`
		Password string `json:"password" minLength:"8"`
	}
}

func (h *Handler) Register(ctx context.Context, input *RegisterInput) (*TokenOutput, error) {
	_, tokens, err := h.auth.Register(
		ctx, service.RegisterInput{
			Name:     input.Body.Name,
			Email:    input.Body.Email,
			Password: input.Body.Password,
		},
	)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			return nil, huma.Error409Conflict("email already registered")
		}
		h.log.Error("register failed", "error", err)
		return nil, huma.Error500InternalServerError("registration failed")
	}

	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		setRefreshCookie(hc, tokens.RefreshToken)
	}

	out := &TokenOutput{}
	out.Body.AccessToken = tokens.AccessToken
	out.Body.TokenType = tokenTypeBearer
	out.Body.ExpiresIn = 15 * 60
	out.Body.ExpiresAt = time.Now().Add(15 * time.Minute)
	return out, nil
}

// ── Login ─────────────────────────────────────────────────────────────────────

type LoginInput struct {
	Body struct {
		Email    string `json:"email"    format:"email"`
		Password string `json:"password"`
	}
}

func (h *Handler) Login(ctx context.Context, input *LoginInput) (*TokenOutput, error) {
	_, tokens, err := h.auth.Login(
		ctx, service.LoginInput{
			Email:    input.Body.Email,
			Password: input.Body.Password,
		},
	)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) || errors.Is(err, service.ErrAccountInactive) {
			return nil, huma.Error401Unauthorized("invalid email or password")
		}
		h.log.Error("login failed", "error", err)
		return nil, huma.Error500InternalServerError("login failed")
	}

	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		setRefreshCookie(hc, tokens.RefreshToken)
	}

	out := &TokenOutput{}
	out.Body.AccessToken = tokens.AccessToken
	out.Body.TokenType = tokenTypeBearer
	out.Body.ExpiresIn = 15 * 60
	out.Body.ExpiresAt = time.Now().Add(15 * time.Minute)
	return out, nil
}

// ── Refresh ───────────────────────────────────────────────────────────────────

type RefreshInput struct{}

func (h *Handler) Refresh(ctx context.Context, _ *RefreshInput) (*TokenOutput, error) {
	hc := middleware.HumaContextFrom(ctx)
	if hc == nil {
		return nil, huma.Error500InternalServerError("context error")
	}

	refreshToken := getRefreshCookie(hc)
	if refreshToken == "" {
		return nil, huma.Error401Unauthorized("missing refresh token")
	}

	tokens, err := h.auth.Refresh(ctx, refreshToken)
	if err != nil {
		clearRefreshCookie(hc)
		return nil, huma.Error401Unauthorized("invalid or expired refresh token")
	}

	setRefreshCookie(hc, tokens.RefreshToken)

	out := &TokenOutput{}
	out.Body.AccessToken = tokens.AccessToken
	out.Body.TokenType = tokenTypeBearer
	out.Body.ExpiresIn = 15 * 60
	out.Body.ExpiresAt = time.Now().Add(15 * time.Minute)
	return out, nil
}

// ── Logout ────────────────────────────────────────────────────────────────────

type LogoutInput struct{}

func (h *Handler) Logout(ctx context.Context, _ *LogoutInput) (*struct{}, error) {
	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		if token := getRefreshCookie(hc); token != "" {
			_ = h.auth.Logout(ctx, token)
		}
		clearRefreshCookie(hc)
	}
	return &struct{}{}, nil
}

// ── Logout All ────────────────────────────────────────────────────────────────

func (h *Handler) LogoutAll(ctx context.Context, _ *struct{}) (*struct{}, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		clearRefreshCookie(hc)
	}

	_ = h.auth.LogoutAll(ctx, claims.UserID)
	return &struct{}{}, nil
}
