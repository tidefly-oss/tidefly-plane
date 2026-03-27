package mapper

import "github.com/tidefly-oss/tidefly-plane/internal/models"

type IntegrationResponse struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Provider   string   `json:"provider"`
	BaseURL    string   `json:"base_url,omitempty"`
	AuthType   string   `json:"auth_type"`
	IsOwner    bool     `json:"is_owner"`
	ProjectIDs []string `json:"project_ids"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

func ToIntegrationResponse(m *models.GitIntegration, currentUserID string) IntegrationResponse {
	isOwner := m.UserID == currentUserID
	projectIDs := make([]string, 0, len(m.Shares))
	if isOwner {
		for _, s := range m.Shares {
			projectIDs = append(projectIDs, s.ProjectID)
		}
	}
	return IntegrationResponse{
		ID: m.ID, Name: m.Name, Provider: m.Provider, BaseURL: m.BaseURL,
		AuthType: m.AuthType, IsOwner: isOwner, ProjectIDs: projectIDs,
		CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func ToIntegrationResponses(integrations []models.GitIntegration, currentUserID string) []IntegrationResponse {
	res := make([]IntegrationResponse, len(integrations))
	for i := range integrations {
		res[i] = ToIntegrationResponse(&integrations[i], currentUserID)
	}
	return res
}
