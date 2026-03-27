package http

import (
	applogger "github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/metrics"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

type Handler struct {
	runtime runtime.Runtime
	log     *applogger.Logger
	metrics *metrics.Registry
}

func New(rt runtime.Runtime, log *applogger.Logger, metricsReg *metrics.Registry) *Handler {
	return &Handler{runtime: rt, log: log, metrics: metricsReg}
}
