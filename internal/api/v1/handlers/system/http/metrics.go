package http

import (
	"context"

	"github.com/tidefly-oss/tidefly-plane/internal/metrics"
)

type MetricsInput struct{}

type MetricsOutput struct {
	Body metrics.SystemSnapshot
}

func (h *Handler) Metrics(_ context.Context, _ *MetricsInput) (*MetricsOutput, error) {
	return &MetricsOutput{Body: h.metrics.GetSystem()}, nil
}
