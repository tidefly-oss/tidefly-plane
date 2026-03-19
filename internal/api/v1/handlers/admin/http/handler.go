package http

import (
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"gorm.io/gorm"
)

type Handler struct {
	users    *service.UserService
	settings *service.SettingsService
	notifier *notifiersvc.Service
	log      *logger.Logger
}

func New(db *gorm.DB, log *logger.Logger, notifier *notifiersvc.Service) *Handler {
	return &Handler{
		users:    service.NewUserService(db),
		settings: service.NewSettingsService(db),
		notifier: notifier,
		log:      log,
	}
}
