package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/containers/service"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	runtime  runtime.Runtime
	deployer *deploy.Deployer
	projects *service.ProjectService
	access   *service.AccessService
	db       *gorm.DB
	log      *logger.Logger
	caddy    *caddysvc.Client
	gitSvc   *git.Service
}

func New(
	rt runtime.Runtime,
	deployer *deploy.Deployer,
	db *gorm.DB,
	log *logger.Logger,
	caddy *caddysvc.Client,
	gitSvc *git.Service,
) *Handler {
	return &Handler{
		runtime:  rt,
		deployer: deployer,
		projects: service.NewProjectService(db),
		access:   service.NewAccessService(db),
		db:       db,
		log:      log,
		caddy:    caddy,
		gitSvc:   gitSvc,
	}
}

func (h *Handler) CaddyEnabled() bool {
	return h.caddy != nil
}
