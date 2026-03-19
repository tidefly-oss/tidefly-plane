package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type MetricsService struct {
	db *gorm.DB
}

func New(db *gorm.DB) *MetricsService {
	return &MetricsService{db: db}
}

func (s *MetricsService) Fetch(ctx context.Context, hours int) ([]models.SystemMetric, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	var metrics []models.SystemMetric
	if err := s.db.WithContext(ctx).
		Where("collected_at >= ?", since).
		Order("collected_at ASC").
		Find(&metrics).Error; err != nil {
		return nil, fmt.Errorf("fetch metrics: %w", err)
	}
	return metrics, nil
}
