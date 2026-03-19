package http

import (
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/containers/service"
	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
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
	traefik  *config.TraefikConfig
}

func New(
	rt runtime.Runtime,
	deployer *deploy.Deployer,
	db *gorm.DB,
	log *logger.Logger,
	traefik *config.TraefikConfig,
) *Handler {
	if traefik == nil {
		traefik = &config.TraefikConfig{}
	}
	return &Handler{
		runtime:  rt,
		deployer: deployer,
		projects: service.NewProjectService(db),
		access:   service.NewAccessService(db),
		db:       db,
		log:      log,
		traefik:  traefik,
	}
}
