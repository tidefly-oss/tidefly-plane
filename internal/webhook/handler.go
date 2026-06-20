package webhook

import (
	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	store *Store
	svc   *Service
	queue *asynq.Client
	log   *logger.Logger
}

func NewHandler(db *gorm.DB, queue *asynq.Client, log *logger.Logger, svc *Service) *Handler {
	return &Handler{
		store: NewStore(db),
		svc:   svc,
		queue: queue,
		log:   log,
	}
}
