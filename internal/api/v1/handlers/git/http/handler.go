package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/git/service"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	gitsvc "github.com/tidefly-oss/tidefly-plane/internal/services/git"
	"gorm.io/gorm"
)

type Handler struct {
	svc         *gitsvc.Service
	integration *service.IntegrationService
	log         *logger.Logger
}

func New(svc *gitsvc.Service, db *gorm.DB, log *logger.Logger) *Handler {
	return &Handler{
		svc:         svc,
		integration: service.New(db),
		log:         log,
	}
}
