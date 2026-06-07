// Package http provides HTTP handlers for system-level endpoints including
// version checks, self-update, and runtime information.
package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

type Handler struct {
	runtime runtime.Runtime
	log     *applogger.Logger
	metrics *metrics.Registry
	bus     *eventbus.Bus
}

func New(rt runtime.Runtime, log *applogger.Logger, metricsReg *metrics.Registry, bus *eventbus.Bus) *Handler {
	return &Handler{runtime: rt, log: log, metrics: metricsReg, bus: bus}
}
