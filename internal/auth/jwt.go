package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrTokenExpired    = errors.New("token expired")
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidHash     = errors.New("invalid hash format")
)

// ── Argon2id params ───────────────────────────────────────────────────────────

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLen     uint32
	keyLen      uint32
}

var defaultArgon2Params = argon2Params{
	memory:      64 * 1024,
	iterations:  3,
	parallelism: 2,
	saltLen:     16,
	keyLen:      32,
}

// ── Claims ────────────────────────────────────────────────────────────────────

type Claims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// ── JWTService ────────────────────────────────────────────────────────────────

type JWTService struct {
	signingKey      []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func NewJWTService(signingKey string) *JWTService {
	return &JWTService{
		signingKey:      []byte(signingKey),
		accessTokenTTL:  15 * time.Minute,
		refreshTokenTTL: 7 * 24 * time.Hour,
	}
}

func (s *JWTService) SigningKey() []byte { return s.signingKey }

// ── Password hashing ──────────────────────────────────────────────────────────

func HashPassword(password string) (string, error) {
	p := defaultArgon2Params
	salt := make([]byte, p.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(password, encodedHash string) error {
	p, salt, hash, err := decodeHash(encodedHash)
	if err != nil {
		return ErrInvalidHash
	}
	computed := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLen)
	if subtle.ConstantTimeCompare(hash, computed) != 1 {
		return ErrInvalidPassword
	}
	return nil
}

func decodeHash(encoded string) (*argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return nil, nil, nil, ErrInvalidHash
	}
	p := &argon2Params{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	p.saltLen = uint32(len(salt))
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	p.keyLen = uint32(len(hash))
	return p, salt, hash, nil
}

// ── JWT ───────────────────────────────────────────────────────────────────────

func (s *JWTService) GenerateAccessToken(userID, email, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID, Email: email, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)),
			Issuer:    "tidefly-plane",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.signingKey)
}

func (s *JWTService) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return s.signingKey, nil
		}, jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ── Token generators ──────────────────────────────────────────────────────────

func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func GenerateWorkerRegistrationToken() (string, error) {
	id := uuid.New().String()
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("tfy_reg_%s_%s", id[:8], base64.URLEncoding.EncodeToString(b)[:16]), nil
}

func GenerateAPIToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("tfy_%s", base64.URLEncoding.EncodeToString(b)), nil
}

// ── TokenStore — PostgreSQL backed ───────────────────────────────────────────
// Replaces the Redis implementation. Refresh tokens are stored in
// the refresh_tokens table. Same semantics, zero extra dependencies.

const refreshTokenTTL = 7 * 24 * time.Hour

// refreshToken is the GORM model for the refresh_tokens table.
type refreshToken struct {
	Token     string    `gorm:"primaryKey"`
	UserID    string    `gorm:"index;not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
}

func (refreshToken) TableName() string { return "refresh_tokens" }

type TokenStore struct {
	db *gorm.DB
}

func NewTokenStore(db *gorm.DB) *TokenStore {
	_ = db.AutoMigrate(&refreshToken{})
	return &TokenStore{db: db}
}

func (s *TokenStore) StoreRefreshToken(ctx context.Context, token, userID string) error {
	return s.db.WithContext(ctx).Create(&refreshToken{
		Token:     token,
		UserID:    userID,
		ExpiresAt: time.Now().UTC().Add(refreshTokenTTL),
	}).Error
}

func (s *TokenStore) ValidateRefreshToken(ctx context.Context, token string) (string, error) {
	var rt refreshToken
	err := s.db.WithContext(ctx).
		Where("token = ? AND expires_at > ?", token, time.Now().UTC()).
		First(&rt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrInvalidToken
		}
		return "", fmt.Errorf("db get: %w", err)
	}
	return rt.UserID, nil
}

func (s *TokenStore) RevokeRefreshToken(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).
		Where("token = ?", token).
		Delete(&refreshToken{}).Error
}

func (s *TokenStore) RevokeAllUserTokens(ctx context.Context, userID string) error {
	return s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&refreshToken{}).Error
}

// ExtendRefreshToken verlängert die Laufzeit eines Tokens.
func (s *TokenStore) ExtendRefreshToken(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).
		Model(&refreshToken{}).
		Where("token = ?", token).
		Update("expires_at", time.Now().UTC().Add(refreshTokenTTL)).Error
}

// Cleanup löscht abgelaufene Tokens — wird vom RetentionWorker aufgerufen.
func (s *TokenStore) Cleanup(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Where("expires_at <= ?", time.Now().UTC()).
		Delete(&refreshToken{}).Error
}
