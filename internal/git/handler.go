package git

import (
	"context"

	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc   *Service
	store *Store
	log   *logger.Logger
	bus   *eventbus.Bus
}

func NewHandler(svc *Service, db *gorm.DB, log *logger.Logger, bus *eventbus.Bus) *Handler {
	return &Handler{
		svc:   svc,
		store: NewStore(db),
		log:   log,
		bus:   bus,
	}
}

func currentUser(ctx context.Context) *models.User {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil
	}
	return &models.User{
		ID:   claims.UserID,
		Role: models.UserRole(claims.Role),
	}
}
