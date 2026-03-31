package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/deploy/service"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/services/agent"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-plane/internal/services/notifier"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/services/template"
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
