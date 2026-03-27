package service

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type IntegrationService struct {
	db *gorm.DB
}

func New(db *gorm.DB) *IntegrationService {
	return &IntegrationService{db: db}
}

func (s *IntegrationService) VisibleIDs(userID string, isAdmin bool) ([]string, error) {
	var ownedIDs []string
	if err := s.db.Model(&models.GitIntegration{}).
		Where("user_id = ?", userID).Pluck("id", &ownedIDs).Error; err != nil {
		return nil, err
	}
	var sharedIDs []string
	var err error
	if isAdmin {
		err = s.db.Raw(`SELECT DISTINCT integration_id::text FROM git_integration_shares`).Scan(&sharedIDs).Error
	} else {
		err = s.db.Raw(
			`
			SELECT git_integration_shares.integration_id::text
			FROM git_integration_shares
			JOIN project_members ON project_members.project_id::text = git_integration_shares.project_id::text
			WHERE project_members.user_id::text = ?`, userID,
		).Scan(&sharedIDs).Error
	}
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(ownedIDs)+len(sharedIDs))
	result := make([]string, 0, len(ownedIDs)+len(sharedIDs))
	for _, id := range append(ownedIDs, sharedIDs...) {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result, nil
}

func (s *IntegrationService) RequireOwner(id, userID string) (*models.GitIntegration, error) {
	var m models.GitIntegration
	if err := s.db.Preload("Shares").First(&m, "id = ?", id).Error; err != nil {
		return nil, huma.Error404NotFound("integration not found")
	}
	if m.UserID != userID {
		return nil, huma.Error403Forbidden("not your integration")
	}
	return &m, nil
}

func (s *IntegrationService) LoadVisible(id, userID string, isAdmin bool) (*models.GitIntegration, error) {
	ids, err := s.VisibleIDs(userID, isAdmin)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}
	for _, vid := range ids {
		if vid == id {
			var m models.GitIntegration
			if err := s.db.Preload("Shares").First(&m, "id = ?", id).Error; err != nil {
				return nil, huma.Error404NotFound("integration not found")
			}
			return &m, nil
		}
	}
	return nil, huma.Error404NotFound("integration not found")
}

func (s *IntegrationService) ListVisible(ids []string) ([]models.GitIntegration, error) {
	var integrations []models.GitIntegration
	if err := s.db.Preload("Shares").Where("id IN ?", ids).Find(&integrations).Error; err != nil {
		return nil, fmt.Errorf("fetch integrations: %w", err)
	}
	return integrations, nil
}

func (s *IntegrationService) Create(m *models.GitIntegration) error {
	return s.db.Create(m).Error
}

func (s *IntegrationService) Delete(id string) error {
	s.db.Where("integration_id = ?", id).Delete(&models.GitIntegrationShare{})
	return s.db.Delete(&models.GitIntegration{}, "id = ?", id).Error
}

func (s *IntegrationService) SetShares(integrationID string, projectIDs []string) error {
	return s.db.Transaction(
		func(tx *gorm.DB) error {
			if err := tx.Where("integration_id = ?", integrationID).
				Delete(&models.GitIntegrationShare{}).Error; err != nil {
				return err
			}
			for _, pid := range projectIDs {
				if err := tx.Create(
					&models.GitIntegrationShare{
						IntegrationID: integrationID,
						ProjectID:     pid,
					},
				).Error; err != nil {
					return err
				}
			}
			return nil
		},
	)
}

func (s *IntegrationService) Reload(m *models.GitIntegration) {
	s.db.Preload("Shares").First(m, "id = ?", m.ID)
}
