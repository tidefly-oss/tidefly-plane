package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/deploy/service"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/template"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/agent"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	deploy      *service.DeployService
	credentials *service.CredentialsService
	notifSvc    *notification.Service
	notifierSvc *notification.Notifier
	log         *logger.Logger
}

func New(
	db *gorm.DB,
	deployer *deploy.Deployer,
	loader *template.Loader,
	log *logger.Logger,
	caddy *caddysvc.Client,
	rt runtime.Runtime,
	notifSvc *notification.Service,
	notifierSvc *notification.Notifier,
	agentClient *agentsvc.Client,
) *Handler {
	return &Handler{
		deploy:      service.NewDeployService(db, deployer, loader, caddy, rt, agentClient),
		credentials: service.NewCredentialsService(db),
		notifSvc:    notifSvc,
		notifierSvc: notifierSvc,
		log:         log,
	}
}
