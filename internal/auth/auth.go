package auth

import (
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
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrInvalidToken    = errors.New("invalid token")
	ErrTokenExpired    = errors.New("token expired")
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidHash     = errors.New("invalid hash format")
)

// ── Argon2id params ───────────────────────────────────────────────────────────
// OWASP recommended minimums for Argon2id (2024)

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLen     uint32
	keyLen      uint32
}

var defaultArgon2Params = argon2Params{
	memory:      64 * 1024, // 64 MB
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

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	signingKey      []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

func New(signingKey string) *Service {
	return &Service{
		signingKey:      []byte(signingKey),
		accessTokenTTL:  15 * time.Minute,
		refreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// ── Password hashing ──────────────────────────────────────────────────────────

// HashPassword hashes a plaintext password using Argon2id.
// Returns a PHC-formatted string: $argon2id$v=19$m=...,t=...,p=...$salt$hash
func HashPassword(password string) (string, error) {
	p := defaultArgon2Params

	salt := make([]byte, p.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		p.memory,
		p.iterations,
		p.parallelism,
		b64Salt,
		b64Hash,
	)
	return encoded, nil
}

// VerifyPassword checks a plaintext password against a stored Argon2id hash.
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
	// $argon2id$v=19$m=65536,t=3,p=2$salt$hash → 6 parts after split
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return nil, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
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

// GenerateAccessToken creates a signed JWT access token (15min TTL).
func (s *Service) GenerateAccessToken(userID, email, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTokenTTL)),
			Issuer:    "tidefly",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.signingKey)
}

// ValidateAccessToken validates a JWT and returns its claims.
func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
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

// GenerateRefreshToken creates a cryptographically random refresh token ID.
// This is stored in Redis, not a JWT — so it's fully revocable.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating refresh token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateWorkerRegistrationToken creates a one-time worker registration token.
func GenerateWorkerRegistrationToken() (string, error) {
	id := uuid.New().String()
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("tfy_reg_%s_%s", id[:8], base64.URLEncoding.EncodeToString(b)[:16]), nil
}

// GenerateAPIToken creates a long-lived API token for Enterprise/CLI use.
func GenerateAPIToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("tfy_%s", base64.URLEncoding.EncodeToString(b)), nil
}
