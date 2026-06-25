package git

import (
	"context"

	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"
)

type Handler struct {
	svc   *Service
	store *Store
	log   *_logger.Logger
	bus   *_eventbus.Bus
}

func NewHandler(svc *Service, db *gorm.DB, log *_logger.Logger, bus *_eventbus.Bus) *Handler {
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
