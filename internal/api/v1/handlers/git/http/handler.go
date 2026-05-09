package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/git/service"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc         *git.Service
	integration *service.IntegrationService
	log         *logger.Logger
}

func New(svc *git.Service, db *gorm.DB, log *logger.Logger) *Handler {
	return &Handler{
		svc:         svc,
		integration: service.New(db),
		log:         log,
	}
}
