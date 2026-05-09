package ca

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

const (
	tokenPrefix     = "tfy_reg_"
	tokenValidHours = 24
	tokenLength     = 32
)

func (s *Service) CreateRegistrationToken(createdByUserID string, label string) (*models.WorkerRegistrationToken, error) {
	raw, err := generateTokenString(tokenLength)
	if err != nil {
		return nil, fmt.Errorf("ca: generate token: %w", err)
	}

	token := &models.WorkerRegistrationToken{
		Token:           tokenPrefix + raw,
		ExpiresAt:       time.Now().Add(tokenValidHours * time.Hour),
		Label:           label,
		CreatedByUserID: createdByUserID,
	}

	if err := s.db.Create(token).Error; err != nil {
		return nil, fmt.Errorf("ca: save registration token: %w", err)
	}

	return token, nil
}

func (s *Service) ConsumeRegistrationToken(tokenValue string, workerID string) (*models.WorkerRegistrationToken, error) {
	var token models.WorkerRegistrationToken
	if err := s.db.Where("token = ?", tokenValue).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("ca: invalid registration token")
		}
		return nil, fmt.Errorf("ca: look up token: %w", err)
	}

	if !token.IsValid() {
		if token.Used {
			return nil, fmt.Errorf("ca: registration token already used")
		}
		return nil, fmt.Errorf("ca: registration token expired")
	}

	now := time.Now()
	if err := s.db.Model(&token).Updates(map[string]any{
		"used":      true,
		"used_at":   now,
		"worker_id": workerID,
	}).Error; err != nil {
		return nil, fmt.Errorf("ca: consume token: %w", err)
	}
	token.Used = true
	token.UsedAt = &now
	token.WorkerID = &workerID

	return &token, nil
}

func (s *Service) ListRegistrationTokens(createdByUserID string) ([]models.WorkerRegistrationToken, error) {
	var tokens []models.WorkerRegistrationToken
	err := s.db.Where("created_by_user_id = ?", createdByUserID).
		Order("created_at DESC").
		Find(&tokens).Error
	return tokens, err
}

func generateTokenString(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
