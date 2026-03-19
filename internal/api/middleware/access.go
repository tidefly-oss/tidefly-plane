package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"gorm.io/gorm"
)

// ContainerAccessDB is the minimal DB interface needed for access checks.
// Pass the *gorm.DB from the handler constructor.

// CheckContainerAccess RequireContainerAccess returns a 403 for Members who don't own the container's project.
//
// Rules:
//   - Admins → always allowed
//   - Members → allowed only if the container belongs to one of their projects
//
// The check works by looking up the project whose label "tidefly.project_id" matches,
// then verifying the user is a member of that project.
//
// Usage inside a handler (not as route middleware, because we need the container label):
//
//	if err := middleware.CheckContainerAccess(c, db, containerLabels); err != nil {
//	    return err
//	}
func CheckContainerAccess(c *echo.Context, db *gorm.DB, labels map[string]string) error {
	user := UserFromContext(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "unauthorized"})
	}
	if user.IsAdmin() {
		return nil
	}
	if err := checkProjectMembership(db, user.ID, labels); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"message": err.Error()})
	}
	return nil
}

func CheckContainerAccessHuma(ctx context.Context, db *gorm.DB, labels map[string]string) error {
	u := UserFromHumaCtx(ctx)
	if u == nil {
		return huma.Error401Unauthorized("unauthorized")
	}
	user, ok := u.(*models.User)
	if !ok {
		return huma.Error401Unauthorized("unauthorized")
	}
	if user.IsAdmin() {
		return nil
	}
	if err := checkProjectMembership(db, user.ID, labels); err != nil {
		return huma.Error403Forbidden(err.Error())
	}
	return nil
}

func checkProjectMembership(db *gorm.DB, userID string, labels map[string]string) error {
	projectID, exists := labels["tidefly.project_id"]
	if !exists || projectID == "" {
		return fmt.Errorf("access denied: container is not part of any project")
	}
	var count int64
	if err := db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("access check failed")
	}
	if count == 0 {
		return fmt.Errorf("access denied: you are not a member of this container's project")
	}
	return nil
}
