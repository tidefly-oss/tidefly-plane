package http

import (
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/webhooks/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	webhooksvc "github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
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
