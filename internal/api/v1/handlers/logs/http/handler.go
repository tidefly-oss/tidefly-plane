package http

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/logs/service"
	"gorm.io/gorm"
)

type Handler struct {
	logs *service.LogService
}

func New(db *gorm.DB) *Handler {
	return &Handler{logs: service.New(db)}
}
