package filter

import (
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

func AllowedNetworks(db *gorm.DB, userID string) (map[string]struct{}, error) {
	var members []models.ProjectMember
	if err := db.Where("user_id = ?", userID).Find(&members).Error; err != nil {
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
	if err := db.Select("network_name").Where("id IN ?", projectIDs).Find(&projects).Error; err != nil {
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

func ContainerAllowed(networks []string, allowed map[string]struct{}) bool {
	for _, n := range networks {
		if _, ok := allowed[n]; ok {
			return true
		}
	}
	return false
}
