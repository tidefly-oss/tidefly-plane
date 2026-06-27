package agent

import (
	"github.com/tidefly-oss/tidefly-plane/internal/platform/ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc         *Service
	agentClient *Client
	bus         *eventbus.Bus
	log         *logger.Logger
}

func NewHandler(db *gorm.DB, caService *ca.Service, agentClient *Client, bus *eventbus.Bus, log *logger.Logger) *Handler {
	return &Handler{
		svc:         NewService(NewStore(db), caService),
		agentClient: agentClient,
		bus:         bus,
		log:         log,
	}
}
