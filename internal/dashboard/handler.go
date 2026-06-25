package dashboard

import (
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"
)

// Handler aggregates data from multiple domains into a single overview response.
// Dependencies are injected directly — no cross-handler calls.
type Handler struct {
	runtime  runtime.Runtime
	db       *gorm.DB
	log      *_logger.Logger
	notifSvc *notification.Service
}

func NewHandler(rt runtime.Runtime, db *gorm.DB, log *_logger.Logger, notifSvc *notification.Service) *Handler {
	return &Handler{
		runtime:  rt,
		db:       db,
		log:      log,
		notifSvc: notifSvc,
	}
}
