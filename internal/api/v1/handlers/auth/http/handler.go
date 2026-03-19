package http

import (
	"github.com/aarondl/authboss/v3"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/auth/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
)

type Handler struct {
	auth *service.AuthService
	log  *logger.Logger
}

func New(db *gorm.DB, ab *authboss.Authboss, log *logger.Logger) *Handler {
	return &Handler{
		auth: service.New(db, ab),
		log:  log,
	}
}
