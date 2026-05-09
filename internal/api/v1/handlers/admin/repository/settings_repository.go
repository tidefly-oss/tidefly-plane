package repository

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type SettingsRepository struct {
	db *gorm.DB
}

func NewSettingsRepository(db *gorm.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get() (models.SystemSettings, error) {
	var s models.SystemSettings
	if err := r.db.First(&s).Error; err != nil {
		return models.SystemSettings{}, nil
	}
	return s, nil
}

func (r *SettingsRepository) Save(s *models.SystemSettings) error {
	if err := r.db.Save(s).Error; err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

func (r *SettingsRepository) FirstOrCreate(s *models.SystemSettings) error {
	if err := r.db.FirstOrCreate(s).Error; err != nil {
		return fmt.Errorf("first or create settings: %w", err)
	}
	return nil
}
