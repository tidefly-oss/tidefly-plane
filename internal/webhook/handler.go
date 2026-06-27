package webhook

import (
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	store *Store
	svc   *Service
	queue *river.Client[pgx.Tx]
	log   *logger.Logger
}

func NewHandler(db *gorm.DB, queue *river.Client[pgx.Tx], log *logger.Logger, svc *Service) *Handler {
	return &Handler{
		store: NewStore(db),
		svc:   svc,
		queue: queue,
		log:   log,
	}
}
