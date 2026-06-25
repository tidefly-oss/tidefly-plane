package admin

import (
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"
)

type Handler struct {
	users    *UserService
	settings *SettingsService
	notifier *notification.Notifier
	log      *_logger.Logger
}

func NewHandler(db *gorm.DB, log *_logger.Logger, notifier *notification.Notifier, caddy *caddysvc.Client) *Handler {
	store := NewStore(db)
	return &Handler{
		users:    NewUserService(store),
		settings: NewSettingsService(store, caddy),
		notifier: notifier,
		log:      log,
	}
}
