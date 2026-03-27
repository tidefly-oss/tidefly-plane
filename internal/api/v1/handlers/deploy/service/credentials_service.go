package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type CredentialsService struct {
	db *gorm.DB
}

func NewCredentialsService(db *gorm.DB) *CredentialsService {
	return &CredentialsService{db: db}
}

func (s *CredentialsService) MarkShown(id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid service id: %w", err)
	}
	if err := s.db.Model(&models.ServiceCredential{}).
		Where("service_id = ? AND plaintext_shown_at IS NULL", parsed).
		Update("plaintext_shown_at", time.Now()).Error; err != nil {
		return fmt.Errorf("mark credentials shown: %w", err)
	}
	return nil
}
