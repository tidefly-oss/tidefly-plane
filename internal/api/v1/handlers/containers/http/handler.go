package http

import (
	"github.com/olahol/melody"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/containers/service"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	runtime  runtime.Runtime
	projects *service.ProjectService
	access   *service.AccessService
	db       *gorm.DB
	log      *logger.Logger
	caddy    *caddysvc.Client
	bus      *eventbus.Bus
	execMel  *melody.Melody
}

func New(
	rt runtime.Runtime,
	db *gorm.DB,
	log *logger.Logger,
	caddy *caddysvc.Client,
	bus *eventbus.Bus,
) *Handler {
	m := melody.New()
	m.Config.MaxMessageSize = 32 * 1024
	h := &Handler{
		runtime:  rt,
		projects: service.NewProjectService(db),
		access:   service.NewAccessService(db),
		db:       db,
		log:      log,
		caddy:    caddy,
		bus:      bus,
		execMel:  m,
	}
	h.setupExecHandlers()
	return h
}

func (h *Handler) CaddyEnabled() bool {
	return h.caddy != nil
}
