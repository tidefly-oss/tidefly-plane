package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/service"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	users    *service.UserService
	settings *service.SettingsService
	notifier *notification.Notifier
	log      *logger.Logger
}

func New(db *gorm.DB, log *logger.Logger, notifier *notification.Notifier, caddy *caddysvc.Client) *Handler {
	return &Handler{
		users:    service.NewUserService(db),
		settings: service.NewSettingsService(db, caddy),
		notifier: notifier,
		log:      log,
	}
}
