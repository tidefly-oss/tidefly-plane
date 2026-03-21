package http

import (
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/containers/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	caddysvc "github.com/tidefly-oss/tidefly-backend/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
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
}

func New(
	rt runtime.Runtime,
	deployer *deploy.Deployer,
	db *gorm.DB,
	log *logger.Logger,
	caddy *caddysvc.Client,
) *Handler {
	return &Handler{
		runtime:  rt,
		deployer: deployer,
		projects: service.NewProjectService(db),
		access:   service.NewAccessService(db),
		db:       db,
		log:      log,
		caddy:    caddy,
	}
}

// CaddyEnabled returns true if Caddy integration is configured.
func (h *Handler) CaddyEnabled() bool {
	return h.caddy != nil
}
