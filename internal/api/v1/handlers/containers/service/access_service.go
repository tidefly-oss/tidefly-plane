package service

import (
	"context"
	"fmt"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	containerfil "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/containers/filter"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
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

// FilterContainers removes internal containers and applies project-based access control.
// Admins see all non-internal containers.
// Members see only containers belonging to their projects.
func (s *AccessService) FilterContainers(ctx context.Context, list []runtime.Container) ([]runtime.Container, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	visible := make([]runtime.Container, 0, len(list))
	for _, c := range list {
		if c.Labels["tidefly-plane.internal"] == "true" {
			continue
		}
		visible = append(visible, c)
	}

	if claims.Role == string(models.RoleAdmin) {
		return visible, nil
	}

	allowed, err := containerfil.AllowedNetworks(s.db, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}

	filtered := make([]runtime.Container, 0, len(visible))
	for _, c := range visible {
		if containerfil.ContainerAllowed(c.Networks, allowed) {
			filtered = append(filtered, c)
		}
	}

	return filtered, nil
}
