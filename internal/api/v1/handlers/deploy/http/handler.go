package http

import (
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/deploy/service"
	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"github.com/tidefly-oss/tidefly-backend/internal/services/template"
	"gorm.io/gorm"
)

type Handler struct {
	deploy      *service.DeployService
	credentials *service.CredentialsService
	notifSvc    *notifications.Service
	notifierSvc *notifiersvc.Service
	log         *logger.Logger
}

func New(
	db *gorm.DB,
	deployer *deploy.Deployer,
	loader *template.Loader,
	log *logger.Logger,
	traefik *config.TraefikConfig,
	notifSvc *notifications.Service,
	notifierSvc *notifiersvc.Service,
) *Handler {
	if traefik == nil {
		traefik = &config.TraefikConfig{}
	}
	return &Handler{
		deploy:      service.NewDeployService(db, deployer, loader, traefik),
		credentials: service.NewCredentialsService(db),
		notifSvc:    notifSvc,
		notifierSvc: notifierSvc,
		log:         log,
	}
}
