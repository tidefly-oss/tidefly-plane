package middleware

import (
	"net/http"

	"github.com/labstack/echo/v5"
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

	// Admins have unrestricted access.
	if user.IsAdmin() {
		return nil
	}

	// For members: resolve the project this container belongs to.
	projectID, ok := labels["tidefly.project_id"]
	if !ok || projectID == "" {
		// Container has no project label → not a project container → members cannot access it.
		return c.JSON(
			http.StatusForbidden, map[string]string{
				"message": "access denied: container is not part of any project",
			},
		)
	}

	// Check membership via the project_members join table.
	var count int64
	err := db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, user.ID).
		Count(&count).Error
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "access check failed"})
	}
	if count == 0 {
		return c.JSON(
			http.StatusForbidden, map[string]string{
				"message": "access denied: you are not a member of this container's project",
			},
		)
	}

	return nil
}
