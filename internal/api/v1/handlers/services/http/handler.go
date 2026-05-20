package http

import (
	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/services/service"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/template"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc        *service.ServiceManager
	log        *applogger.Logger
	templateLd *template.Loader
}

func New(
	db *gorm.DB,
	deployer *deploy.Deployer,
	queue *asynq.Client,
	log *applogger.Logger,
	gitSvc *git.Service,
	templateLd *template.Loader,
	rt runtime.Runtime,
	ingressAdapter ingress.Adapter,
) *Handler {
	return &Handler{
		svc:        service.New(db, deployer, queue, log, gitSvc, rt, ingressAdapter),
		log:        log,
		templateLd: templateLd,
	}
}
