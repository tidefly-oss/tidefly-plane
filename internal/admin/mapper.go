package admin

import (
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type UserResponse struct {
	ID                  string          `json:"id"`
	Email               string          `json:"email"`
	Name                string          `json:"name"`
	Role                models.UserRole `json:"role"`
	Active              bool            `json:"active"`
	ForcePasswordChange bool            `json:"force_password_change"`
	CreatedAt           time.Time       `json:"created_at"`
	ProjectIDs          []string        `json:"project_ids"`
}

func toUserResponse(u *models.User) UserResponse {
	ids := make([]string, 0, len(u.ProjectMembers))
	for _, pm := range u.ProjectMembers {
		ids = append(ids, pm.ProjectID)
	}
	return UserResponse{
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

func toUserResponses(users []models.User) []UserResponse {
	res := make([]UserResponse, len(users))
	for i := range users {
		res[i] = toUserResponse(&users[i])
	}
	return res
}
