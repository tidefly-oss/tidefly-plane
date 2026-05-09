package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	refreshTokenPrefix = "refresh:"
	refreshTokenTTL    = 7 * 24 * time.Hour
)

// TokenStore manages refresh tokens in Redis.
// Refresh tokens are random IDs (not JWTs) — fully revocable by deleting the key.
type TokenStore struct {
	rdb *redis.Client
}

func NewTokenStore(rdb *redis.Client) *TokenStore {
	return &TokenStore{rdb: rdb}
}

// StoreRefreshToken saves a refresh token → userID mapping in Redis.
func (s *TokenStore) StoreRefreshToken(ctx context.Context, token, userID string) error {
	key := refreshTokenPrefix + token
	return s.rdb.Set(ctx, key, userID, refreshTokenTTL).Err()
}

// ValidateRefreshToken returns the userID for a given refresh token, or an error.
func (s *TokenStore) ValidateRefreshToken(ctx context.Context, token string) (string, error) {
	key := refreshTokenPrefix + token
	userID, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", ErrInvalidToken
		}
		return "", fmt.Errorf("redis get: %w", err)
	}
	return userID, nil
}

// RevokeRefreshToken deletes a single refresh token (logout).
func (s *TokenStore) RevokeRefreshToken(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, refreshTokenPrefix+token).Err()
}

// RevokeAllUserTokens removes all refresh tokens for a user.
// Used when: password change, account deletion, force-logout.
func (s *TokenStore) RevokeAllUserTokens(ctx context.Context, userID string) error {
	// Scan for all refresh tokens belonging to this user
	// Pattern: we store userID as value, so we need to scan all refresh keys
	// For efficiency at scale this would use a secondary index (Set per user)
	// For Tidefly v1 this scan approach is fine
	var cursor uint64
	for {
		keys, nextCursor, err := s.rdb.Scan(ctx, cursor, refreshTokenPrefix+"*", 100).Result()
		if err != nil {
			return fmt.Errorf("redis scan: %w", err)
		}

		for _, key := range keys {
			val, err := s.rdb.Get(ctx, key).Result()
			if err != nil {
				continue
			}
			if val == userID {
				s.rdb.Del(ctx, key)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

// ExtendRefreshToken resets the TTL on a refresh token (sliding window).
func (s *TokenStore) ExtendRefreshToken(ctx context.Context, token string) error {
	key := refreshTokenPrefix + token
	return s.rdb.Expire(ctx, key, refreshTokenTTL).Err()
}
