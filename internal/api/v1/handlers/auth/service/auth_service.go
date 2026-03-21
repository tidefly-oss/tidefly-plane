package service

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/auth"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountInactive    = errors.New("account is inactive")
)

// ── TokenPair ─────────────────────────────────────────────────────────────────

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// ── RegisterInput / LoginInput ────────────────────────────────────────────────

type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

// ── AuthService ───────────────────────────────────────────────────────────────

type AuthService struct {
	db         *gorm.DB
	jwt        *auth.Service
	tokenStore *auth.TokenStore
}

func New(db *gorm.DB, jwtSvc *auth.Service, tokenStore *auth.TokenStore) *AuthService {
	return &AuthService{db: db, jwt: jwtSvc, tokenStore: tokenStore}
}

// ── GetFreshUser ──────────────────────────────────────────────────────────────
// Unchanged signature — still preloads ProjectMembers.

func (s *AuthService) GetFreshUser(id string) (models.User, error) {
	var u models.User
	if err := s.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

// ── ChangePassword ────────────────────────────────────────────────────────────
// Replaces authboss hasher with Argon2id from internal/auth.

func (s *AuthService) ChangePassword(user *models.User, currentPassword, newPassword string) error {
	if err := auth.VerifyPassword(currentPassword, user.Password); err != nil {
		return fmt.Errorf("wrong_current_password")
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash_failed")
	}

	if err := s.db.Exec(
		"UPDATE users SET password = ?, force_password_change = false WHERE id = ?",
		hash, user.ID,
	).Error; err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// ── Register ──────────────────────────────────────────────────────────────────

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*models.User, *TokenPair, error) {
	var existing models.User
	err := s.db.Where("email = ?", input.Email).First(&existing).Error
	if err == nil {
		return nil, nil, ErrEmailTaken
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("db lookup: %w", err)
	}

	hashed, err := auth.HashPassword(input.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("hashing password: %w", err)
	}

	// First user ever → admin
	var count int64
	s.db.Model(&models.User{}).Count(&count)
	role := models.RoleMember
	if count == 0 {
		role = models.RoleAdmin
	}

	user := &models.User{
		Name:     input.Name,
		Email:    input.Email,
		Password: hashed,
		Role:     role,
		Active:   true,
	}
	if err := s.db.Create(user).Error; err != nil {
		return nil, nil, fmt.Errorf("creating user: %w", err)
	}

	tokens, err := s.issueTokens(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	return user, tokens, nil
}

// ── Login ─────────────────────────────────────────────────────────────────────

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*models.User, *TokenPair, error) {
	var user models.User
	if err := s.db.Where("email = ?", input.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Timing-attack prevention: always run Argon2id even when user doesn't exist
			_ = auth.VerifyPassword(
				input.Password,
				"$argon2id$v=19$m=65536,t=3,p=2$dGlkZWZseWR1bW15c2FsdA$dGlkZWZseWR1bW15aGFzaDEyMzQ1Njc4OTAxMjM0NTY",
			)
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, fmt.Errorf("db lookup: %w", err)
	}

	if !user.Active {
		return nil, nil, ErrAccountInactive
	}

	if err := auth.VerifyPassword(input.Password, user.Password); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	tokens, err := s.issueTokens(ctx, &user)
	if err != nil {
		return nil, nil, err
	}
	return &user, tokens, nil
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	userID, err := s.tokenStore.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, auth.ErrInvalidToken
	}

	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, ErrUserNotFound
	}
	if !user.Active {
		_ = s.tokenStore.RevokeRefreshToken(ctx, refreshToken)
		return nil, ErrAccountInactive
	}

	// Rotate — revoke old, issue new
	_ = s.tokenStore.RevokeRefreshToken(ctx, refreshToken)
	return s.issueTokens(ctx, &user)
}

// ── Logout ────────────────────────────────────────────────────────────────────

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	return s.tokenStore.RevokeRefreshToken(ctx, refreshToken)
}

func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	return s.tokenStore.RevokeAllUserTokens(ctx, userID)
}

// ── internal ──────────────────────────────────────────────────────────────────

func (s *AuthService) issueTokens(ctx context.Context, user *models.User) (*TokenPair, error) {
	accessToken, err := s.jwt.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	if err := s.tokenStore.StoreRefreshToken(ctx, refreshToken, user.ID); err != nil {
		return nil, fmt.Errorf("storing refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}
