package http

import (
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	caddysvc "github.com/tidefly-oss/tidefly-backend/internal/services/caddy"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"gorm.io/gorm"
)

type Handler struct {
	users    *service.UserService
	settings *service.SettingsService
	notifier *notifiersvc.Service
	log      *logger.Logger
}

func New(db *gorm.DB, log *logger.Logger, notifier *notifiersvc.Service, caddy *caddysvc.Client) *Handler {
	return &Handler{
		users:    service.NewUserService(db),
		settings: service.NewSettingsService(db, caddy),
		notifier: notifier,
		log:      log,
	}
}
