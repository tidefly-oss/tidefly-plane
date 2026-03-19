package http

import (
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/system/service"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	"gorm.io/gorm"
)

type Handler struct {
	runtime runtime.Runtime
	metrics *service.MetricsService
}

func New(rt runtime.Runtime, db *gorm.DB) *Handler {
	return &Handler{
		runtime: rt,
		metrics: service.New(db),
	}
}
