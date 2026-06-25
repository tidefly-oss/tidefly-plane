package project

import (
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc     *Service
	runtime runtime.Runtime
	log     *_logger.Logger
}

func NewHandler(db *gorm.DB, rt runtime.Runtime, log *_logger.Logger) *Handler {
	return &Handler{
		svc:     NewService(db),
		runtime: rt,
		log:     log,
	}
}
