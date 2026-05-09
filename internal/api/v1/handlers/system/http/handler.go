package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

type Handler struct {
	runtime runtime.Runtime
	log     *applogger.Logger
	metrics *metrics.Registry
}

func New(rt runtime.Runtime, log *applogger.Logger, metricsReg *metrics.Registry) *Handler {
	return &Handler{runtime: rt, log: log, metrics: metricsReg}
}
