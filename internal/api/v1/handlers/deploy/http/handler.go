package http

import (
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/deploy/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	caddysvc "github.com/tidefly-oss/tidefly-backend/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
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
	caddy *caddysvc.Client,
	rt runtime.Runtime,
	notifSvc *notifications.Service,
	notifierSvc *notifiersvc.Service,
) *Handler {
	return &Handler{
		deploy:      service.NewDeployService(db, deployer, loader, caddy, rt),
		credentials: service.NewCredentialsService(db),
		notifSvc:    notifSvc,
		notifierSvc: notifierSvc,
		log:         log,
	}
}
