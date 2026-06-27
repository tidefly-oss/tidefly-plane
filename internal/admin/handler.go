package admin

import (
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	users    *UserService
	settings *SettingsService
	notifier *notification.Notifier
	log      *logger.Logger
}

func NewHandler(db *gorm.DB, log *logger.Logger, notifier *notification.Notifier, caddy *caddysvc.Client) *Handler {
	store := NewStore(db)
	return &Handler{
		users:    NewUserService(store),
		settings: NewSettingsService(store, caddy),
		notifier: notifier,
		log:      log,
	}
}
