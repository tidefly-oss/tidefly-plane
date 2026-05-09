package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/auth/service"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	auth *service.AuthService
	jwt  *auth.Service
	log  *logger.Logger
}

func New(db *gorm.DB, jwtSvc *auth.Service, tokenStore *auth.TokenStore, log *logger.Logger) *Handler {
	return &Handler{
		auth: service.New(db, jwtSvc, tokenStore),
		jwt:  jwtSvc,
		log:  log,
	}
}
