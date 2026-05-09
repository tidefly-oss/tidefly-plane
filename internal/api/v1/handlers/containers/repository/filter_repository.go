package repository

import (
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type FilterRepository struct {
	db *gorm.DB
}

func NewFilterRepository(db *gorm.DB) *FilterRepository {
	return &FilterRepository{db: db}
}

func (r *FilterRepository) AllowedNetworks(userID string) (map[string]struct{}, error) {
	var members []models.ProjectMember
	if err := r.db.Where("user_id = ?", userID).Find(&members).Error; err != nil {
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
	if err := r.db.Select("network_name").Where("id IN ?", projectIDs).Find(&projects).Error; err != nil {
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

func (r *FilterRepository) DB() *gorm.DB {
	return r.db
}
