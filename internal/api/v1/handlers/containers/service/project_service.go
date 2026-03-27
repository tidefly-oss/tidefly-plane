package service

import (
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

type ProjectService struct {
	db *gorm.DB
}

func NewProjectService(db *gorm.DB) *ProjectService {
	return &ProjectService{db: db}
}

func (s *ProjectService) GetByID(id string) (models.Project, error) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return models.Project{}, fmt.Errorf("project not found: %w", err)
	}
	return p, nil
}
