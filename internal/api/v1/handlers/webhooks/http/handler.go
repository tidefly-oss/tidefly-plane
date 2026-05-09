package http

import (
	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/webhooks/service"
	webhooksvc "github.com/tidefly-oss/tidefly-plane/internal/domain/webhook"
)

type Handler struct {
	webhooks *service.WebhookService
	queue    *asynq.Client
	log      *logger.Logger
	svc      *webhooksvc.Service
}

func New(db *gorm.DB, queue *asynq.Client, log *logger.Logger, svc *webhooksvc.Service) *Handler {
	return &Handler{
		webhooks: service.New(db),
		queue:    queue,
		log:      log,
		svc:      svc,
	}
}
