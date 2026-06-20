package container

import (
	"github.com/olahol/melody"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	runtime runtime.Runtime
	store   *Store
	access  *accessService
	db      *gorm.DB
	log     *logger.Logger
	caddy   *caddysvc.Client
	bus     *eventbus.Bus
	execMel *melody.Melody
}

func NewHandler(
	rt runtime.Runtime,
	db *gorm.DB,
	log *logger.Logger,
	caddy *caddysvc.Client,
	bus *eventbus.Bus,
) *Handler {
	m := melody.New()
	m.Config.MaxMessageSize = 32 * 1024
	h := &Handler{
		runtime: rt,
		store:   NewStore(db),
		access:  newAccessService(db),
		db:      db,
		log:     log,
		caddy:   caddy,
		bus:     bus,
		execMel: m,
	}
	h.setupExecHandlers()
	return h
}

func (h *Handler) CaddyEnabled() bool {
	return h.caddy != nil
}
