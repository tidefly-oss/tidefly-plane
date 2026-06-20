package auth

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountInactive    = errors.New("account is inactive")
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

type AuthService struct {
	store      *Store
	jwt        *JWTService
	tokenStore *TokenStore
}

func NewAuthService(store *Store, jwtSvc *JWTService, tokenStore *TokenStore) *AuthService {
	return &AuthService{store: store, jwt: jwtSvc, tokenStore: tokenStore}
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*models.User, *TokenPair, error) {
	existing, err := s.store.FindByEmail(input.Email)
	if existing != nil {
		return nil, nil, ErrEmailTaken
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("db lookup: %w", err)
	}

	hashed, err := HashPassword(input.Password)
	if err != nil {
		return nil, nil, fmt.Errorf("hashing password: %w", err)
	}

	count, err := s.store.Count()
	if err != nil {
		return nil, nil, err
	}
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
	if err := s.store.Create(user); err != nil {
		return nil, nil, err
	}

	tokens, err := s.issueTokens(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	return user, tokens, nil
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*models.User, *TokenPair, error) {
	user, err := s.store.FindByEmail(input.Email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Constant-time dummy verify to prevent timing attacks
			_ = VerifyPassword(
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
	if err := VerifyPassword(input.Password, user.Password); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	tokens, err := s.issueTokens(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	return user, tokens, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	userID, err := s.tokenStore.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.store.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	if !user.Active {
		_ = s.tokenStore.RevokeRefreshToken(ctx, refreshToken)
		return nil, ErrAccountInactive
	}

	_ = s.tokenStore.RevokeRefreshToken(ctx, refreshToken)
	return s.issueTokens(ctx, user)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	return s.tokenStore.RevokeRefreshToken(ctx, refreshToken)
}

func (s *AuthService) LogoutAll(ctx context.Context, userID string) error {
	return s.tokenStore.RevokeAllUserTokens(ctx, userID)
}

func (s *AuthService) GetFreshUser(id string) (models.User, error) {
	u, err := s.store.FindByIDWithProjects(id)
	if err != nil {
		return models.User{}, ErrUserNotFound
	}
	return *u, nil
}

func (s *AuthService) ChangePassword(user *models.User, currentPassword, newPassword string) error {
	if err := VerifyPassword(currentPassword, user.Password); err != nil {
		return fmt.Errorf("wrong_current_password")
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash_failed")
	}
	return s.store.UpdatePassword(user, hash, false)
}

func (s *AuthService) issueTokens(ctx context.Context, user *models.User) (*TokenPair, error) {
	accessToken, err := s.jwt.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}
	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}
	if err := s.tokenStore.StoreRefreshToken(ctx, refreshToken, user.ID); err != nil {
		return nil, fmt.Errorf("storing refresh token: %w", err)
	}
	return &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}
