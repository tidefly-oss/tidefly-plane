package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// CheckContainerAccess checks if the current Echo user may access a container.
// Admins always pass. Members must be part of the container's project.
// Signature unchanged.
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

// CheckContainerAccessHuma checks container access from a Huma context.
// Now reads userID from JWT claims instead of authboss.User.
func CheckContainerAccessHuma(ctx context.Context, db *gorm.DB, labels map[string]string) error {
	claims := UserFromHumaCtx(ctx)
	if claims == nil {
		return huma.Error401Unauthorized("unauthorized")
	}

	// Admins pass via role claim — no DB lookup needed
	if claims.Role == string(models.RoleAdmin) {
		return nil
	}

	if err := checkProjectMembership(db, claims.UserID, labels); err != nil {
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
