package http

import (
	"context"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type MetricsInput struct {
	Hours int `query:"hours" minimum:"1" maximum:"168" default:"24"`
}
type MetricsOutput struct {
	Body struct {
		Metrics []models.SystemMetric `json:"metrics"`
		Latest  *models.SystemMetric  `json:"latest"`
	}
}

func (h *Handler) Metrics(ctx context.Context, input *MetricsInput) (*MetricsOutput, error) {
	if input.Hours <= 0 || input.Hours > 168 {
		input.Hours = 24
	}
	metrics, err := h.metrics.Fetch(ctx, input.Hours)
	if err != nil {
		return nil, err
	}
	out := &MetricsOutput{}
	out.Body.Metrics = metrics
	if len(metrics) > 0 {
		out.Body.Latest = &metrics[len(metrics)-1]
	}
	return out, nil
}
