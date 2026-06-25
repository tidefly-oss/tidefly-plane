package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
)

// ── Register ──────────────────────────────────────────────────────────────────

type registerInput struct {
	Body struct {
		Name     string `json:"name"     minLength:"1" maxLength:"255"`
		Email    string `json:"email"    format:"email"`
		Password string `json:"password" minLength:"8"`
	}
}

func (h *Handler) register(ctx context.Context, input *registerInput) (*tokenOutput, error) {
	_, tokens, err := h.svc.Register(ctx, RegisterInput{
		Name:     input.Body.Name,
		Email:    input.Body.Email,
		Password: input.Body.Password,
	})
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return nil, huma.Error409Conflict("email already registered")
		}
		h.log.Error("auth", "register failed", err)
		return nil, huma.Error500InternalServerError("registration failed")
	}
	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		setRefreshCookie(hc, tokens.RefreshToken)
	}
	return newTokenOutput(tokens.AccessToken), nil
}

// ── Login ─────────────────────────────────────────────────────────────────────

type loginInput struct {
	Body struct {
		Email    string `json:"email"    format:"email"`
		Password string `json:"password"`
	}
}

func (h *Handler) login(ctx context.Context, input *loginInput) (*tokenOutput, error) {
	_, tokens, err := h.svc.Login(ctx, LoginInput{
		Email:    input.Body.Email,
		Password: input.Body.Password,
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrAccountInactive) {
			return nil, huma.Error401Unauthorized("invalid email or password")
		}
		h.log.Error("auth", "login failed", err)
		return nil, huma.Error500InternalServerError("login failed")
	}
	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		setRefreshCookie(hc, tokens.RefreshToken)
	}
	return newTokenOutput(tokens.AccessToken), nil
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (h *Handler) refresh(ctx context.Context, _ *struct{}) (*tokenOutput, error) {
	hc := middleware.HumaContextFrom(ctx)
	if hc == nil {
		return nil, huma.Error500InternalServerError("context error")
	}
	refreshToken := getRefreshCookie(hc)
	if refreshToken == "" {
		return nil, huma.Error401Unauthorized("missing refresh token")
	}
	tokens, err := h.svc.Refresh(ctx, refreshToken)
	if err != nil {
		clearRefreshCookie(hc)
		return nil, huma.Error401Unauthorized("invalid or expired refresh token")
	}
	setRefreshCookie(hc, tokens.RefreshToken)
	return newTokenOutput(tokens.AccessToken), nil
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (h *Handler) logout(ctx context.Context, _ *struct{}) (*struct{}, error) {
	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		if token := getRefreshCookie(hc); token != "" {
			_ = h.svc.Logout(ctx, token)
		}
		clearRefreshCookie(hc)
	}
	return &struct{}{}, nil
}

func (h *Handler) logoutAll(ctx context.Context, _ *struct{}) (*struct{}, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if hc := middleware.HumaContextFrom(ctx); hc != nil {
		clearRefreshCookie(hc)
	}
	_ = h.svc.LogoutAll(ctx, claims.UserID)
	return &struct{}{}, nil
}

// ── Me ────────────────────────────────────────────────────────────────────────

type meOutput struct {
	Body struct {
		User CurrentUserResponse `json:"user"`
	}
}

func (h *Handler) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	u, err := h.svc.GetFreshUser(claims.UserID)
	if err != nil {
		return nil, huma.Error401Unauthorized("user not found")
	}
	out := &meOutput{}
	out.Body.User = ToCurrentUserResponse(&u)
	return out, nil
}

// ── Change password ───────────────────────────────────────────────────────────

type changePasswordInput struct {
	Body struct {
		CurrentPassword string `json:"current_password" minLength:"1"`
		NewPassword     string `json:"new_password"     minLength:"8"`
	}
}

type changePasswordOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (h *Handler) changePassword(ctx context.Context, input *changePasswordInput) (*changePasswordOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	u, err := h.svc.GetFreshUser(claims.UserID)
	if err != nil {
		return nil, huma.Error401Unauthorized("user not found")
	}

	err = h.svc.ChangePassword(&u, input.Body.CurrentPassword, input.Body.NewPassword)
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditPasswordChange,
		ResourceID: claims.UserID,
		Success:    err == nil,
		Details:    fmt.Sprintf("email=%s reason=%v", claims.Email, err),
	})

	if err != nil {
		if err.Error() == "wrong_current_password" {
			return nil, huma.NewError(http.StatusUnauthorized, "current password is incorrect")
		}
		return nil, err
	}

	out := &changePasswordOutput{}
	out.Body.Message = "password changed successfully"
	return out, nil
}
