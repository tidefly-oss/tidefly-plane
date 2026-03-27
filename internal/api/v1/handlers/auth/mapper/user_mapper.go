package mapper

import (
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type CurrentUserResponse struct {
	ID                  string          `json:"id"`
	Email               string          `json:"email"`
	Name                string          `json:"name"`
	Role                models.UserRole `json:"role"`
	ForcePasswordChange bool            `json:"force_password_change"`
	ProjectIDs          []string        `json:"project_ids"`
}

func ToCurrentUserResponse(u *models.User) CurrentUserResponse {
	ids := make([]string, 0, len(u.ProjectMembers))
	for _, pm := range u.ProjectMembers {
		ids = append(ids, pm.ProjectID)
	}
	return CurrentUserResponse{
		ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role,
		ForcePasswordChange: u.ForcePasswordChange, ProjectIDs: ids,
	}
}
