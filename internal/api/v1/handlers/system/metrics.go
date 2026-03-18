package system

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type MetricsHandler struct {
	db *gorm.DB
}

func NewMetrics(db *gorm.DB) *MetricsHandler {
	return &MetricsHandler{db: db}
}

type MetricsInput struct {
	Hours int `query:"hours" minimum:"1" maximum:"168" default:"24"`
}
type MetricsOutput struct {
	Body struct {
		Metrics []models.SystemMetric `json:"metrics"`
		Latest  *models.SystemMetric  `json:"latest"`
	}
}

func (h *MetricsHandler) Metrics(ctx context.Context, input *MetricsInput) (*MetricsOutput, error) {
	if input.Hours <= 0 || input.Hours > 168 {
		input.Hours = 24
	}
	since := time.Now().Add(-time.Duration(input.Hours) * time.Hour)

	var metrics []models.SystemMetric
	if err := h.db.WithContext(ctx).
		Where("collected_at >= ?", since).
		Order("collected_at ASC").
		Find(&metrics).Error; err != nil {
		return nil, fmt.Errorf("fetch metrics: %w", err)
	}

	out := &MetricsOutput{}
	out.Body.Metrics = metrics
	if len(metrics) > 0 {
		out.Body.Latest = &metrics[len(metrics)-1]
	}
	return out, nil
}
