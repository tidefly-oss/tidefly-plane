package agent

import (
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc         *Service
	agentClient *Client
	bus         *_eventbus.Bus
	log         *_logger.Logger
}

func NewHandler(db *gorm.DB, caService *_ca.Service, agentClient *Client, bus *_eventbus.Bus, log *_logger.Logger) *Handler {
	return &Handler{
		svc:         NewService(NewStore(db), caService),
		agentClient: agentClient,
		bus:         bus,
		log:         log,
	}
}
