package service

import (
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
)

type AccessService struct {
	db *gorm.DB
}

func NewAccessService(db *gorm.DB) *AccessService {
	return &AccessService{db: db}
}

func (s *AccessService) CheckContainerAccess(c *echo.Context, labels map[string]string) error {
	return middleware.CheckContainerAccess(c, s.db, labels)
}
