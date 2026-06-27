package dashboard

import (
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"gorm.io/gorm"
)

type Handler struct {
	runtime  runtime.Runtime
	db       *gorm.DB
	log      *logger.Logger
	notifSvc *notification.Service
	metrics  *metrics.Registry
}

func NewHandler(rt runtime.Runtime, db *gorm.DB, log *logger.Logger, notifSvc *notification.Service, metricsReg *metrics.Registry) *Handler {
	return &Handler{
		runtime:  rt,
		db:       db,
		log:      log,
		notifSvc: notifSvc,
		metrics:  metricsReg,
	}
}
