// Package http provides the HTTP handler for the dashboard overview aggregation endpoint.
package http

import (
	notificationsvc "github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

// Handler aggregates data from multiple domains into a single overview response.
// Dependencies are injected directly — no cross-handler calls.
type Handler struct {
	runtime  runtime.Runtime
	db       *gorm.DB
	log      *logger.Logger
	notifSvc *notificationsvc.Service
}

func New(
	rt runtime.Runtime,
	db *gorm.DB,
	log *logger.Logger,
	notifSvc *notificationsvc.Service,
) *Handler {
	return &Handler{
		runtime:  rt,
		db:       db,
		log:      log,
		notifSvc: notifSvc,
	}
}
