package system

import (
	"context"

	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

type metricsOutput struct {
	Body metrics.SystemSnapshot
}

func (h *Handler) getMetrics(_ context.Context, _ *struct{}) (*metricsOutput, error) {
	return &metricsOutput{Body: h.metrics.GetSystem()}, nil
}
