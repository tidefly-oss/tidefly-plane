package container

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"gorm.io/gorm"
)

// AllowedNetworks is a convenience wrapper for external callers.
func AllowedNetworks(db *gorm.DB, userID string) (map[string]struct{}, error) {
	return access.NewStore(db).AllowedNetworks(userID)
}

type accessService struct {
	store *access.Store
}

func newAccessService(db *gorm.DB) *accessService {
	return &accessService{store: access.NewStore(db)}
}

func (s *accessService) CheckContainerAccess(ctx context.Context, labels map[string]string) error {
	u := access.CurrentUser(ctx)
	if u == nil {
		return fmt.Errorf("unauthorized")
	}
	if u.Role == "admin" {
		return nil
	}
	if err := access.CheckProjectMembership(s.store.DB(), u.UserID, labels); err != nil {
		return fmt.Errorf("access denied: %w", err)
	}
	return nil
}

func (s *accessService) FilterContainers(ctx context.Context, list []runtime.Container) ([]runtime.Container, error) {
	u := access.CurrentUser(ctx)
	if u == nil {
		return nil, fmt.Errorf("unauthorized")
	}
	visible := make([]runtime.Container, 0, len(list))
	for _, c := range list {
		if access.IsInternal(c.Labels) {
			continue
		}
		visible = append(visible, c)
	}
	if u.Role == "admin" {
		return visible, nil
	}
	allowed, err := s.store.AllowedNetworks(u.UserID)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}
	filtered := make([]runtime.Container, 0, len(visible))
	for _, c := range visible {
		if access.NetworkAllowed(c.Networks, allowed) {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}
