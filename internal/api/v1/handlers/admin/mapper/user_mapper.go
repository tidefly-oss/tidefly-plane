package mapper

import (
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type AdminUserResponse struct {
	ID                  string          `json:"id"`
	Email               string          `json:"email"`
	Name                string          `json:"name"`
	Role                models.UserRole `json:"role"`
	Active              bool            `json:"active"`
	ForcePasswordChange bool            `json:"force_password_change"`
	CreatedAt           time.Time       `json:"created_at"`
	ProjectIDs          []string        `json:"project_ids"`
}

func ToAdminUserResponse(u *models.User) AdminUserResponse {
	ids := make([]string, 0, len(u.ProjectMembers))
	for _, pm := range u.ProjectMembers {
		ids = append(ids, pm.ProjectID)
	}

	return AdminUserResponse{
		ID:                  u.ID,
		Email:               u.Email,
		Name:                u.Name,
		Role:                u.Role,
		Active:              u.Active,
		ForcePasswordChange: u.ForcePasswordChange,
		CreatedAt:           u.CreatedAt,
		ProjectIDs:          ids,
	}
}

func ToAdminUserResponses(users []models.User) []AdminUserResponse {
	res := make([]AdminUserResponse, len(users))
	for i := range users {
		res[i] = ToAdminUserResponse(&users[i])
	}
	return res
}
