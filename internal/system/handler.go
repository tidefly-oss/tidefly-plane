package system

import (
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_eventbus"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

type Handler struct {
	runtime runtime.Runtime
	log     *applogger.Logger
	metrics *metrics.Registry
	bus     *_eventbus.Bus
}

func NewHandler(rt runtime.Runtime, log *applogger.Logger, metricsReg *metrics.Registry, bus *_eventbus.Bus) *Handler {
	return &Handler{runtime: rt, log: log, metrics: metricsReg, bus: bus}
}
