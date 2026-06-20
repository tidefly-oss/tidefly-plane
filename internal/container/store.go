package container

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) AllowedNetworks(userID string) (map[string]struct{}, error) {
	var members []models.ProjectMember
	if err := s.db.Where("user_id = ?", userID).Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return map[string]struct{}{}, nil
	}
	projectIDs := make([]string, len(members))
	for i, m := range members {
		projectIDs[i] = m.ProjectID
	}
	var projects []models.Project
	if err := s.db.Select("network_name").Where("id IN ?", projectIDs).Find(&projects).Error; err != nil {
		return nil, err
	}
	nets := make(map[string]struct{}, len(projects))
	for _, p := range projects {
		if p.NetworkName != "" {
			nets[p.NetworkName] = struct{}{}
		}
	}
	return nets, nil
}

func (s *Store) GetProjectByID(id string) (models.Project, error) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return models.Project{}, fmt.Errorf("project not found: %w", err)
	}
	return p, nil
}
