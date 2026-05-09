package service

import (
	"context"
	"fmt"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/containers/repository"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type AccessService struct {
	repo *repository.FilterRepository
}

func NewAccessService(db *gorm.DB) *AccessService {
	return &AccessService{repo: repository.NewFilterRepository(db)}
}

func (s *AccessService) CheckContainerAccess(c *echo.Context, labels map[string]string) error {
	return middleware.CheckContainerAccess(c, s.repo.DB(), labels)
}

func (s *AccessService) FilterContainers(ctx context.Context, list []runtime.Container) ([]runtime.Container, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, fmt.Errorf("unauthorized")
	}

	visible := make([]runtime.Container, 0, len(list))
	for _, c := range list {
		if c.Labels["tidefly.internal"] == "true" {
			continue
		}
		visible = append(visible, c)
	}

	if claims.Role == string(models.RoleAdmin) {
		return visible, nil
	}

	allowed, err := s.repo.AllowedNetworks(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}

	filtered := make([]runtime.Container, 0, len(visible))
	for _, c := range visible {
		for _, n := range c.Networks {
			if _, ok := allowed[n]; ok {
				filtered = append(filtered, c)
				break
			}
		}
	}

	return filtered, nil
}
