package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type ProjectService struct {
	db *gorm.DB
}

func New(db *gorm.DB) *ProjectService {
	return &ProjectService{db: db}
}

func (s *ProjectService) List() ([]models.Project, error) {
	var list []models.Project
	if err := s.db.Order("created_at desc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return list, nil
}

func (s *ProjectService) GetByID(id string) (models.Project, error) {
	var p models.Project
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return models.Project{}, fmt.Errorf("project not found: %w", err)
	}
	return p, nil
}

func (s *ProjectService) Create(p *models.Project) error {
	return s.db.Create(p).Error
}

type UpdateFields struct {
	Name        string
	Description string
	Color       string
}

func (s *ProjectService) Update(p *models.Project, fields UpdateFields) ([]string, error) {
	updates := map[string]any{}
	var changes []string
	if fields.Name != "" && fields.Name != p.Name {
		updates["name"] = fields.Name
		changes = append(changes, fmt.Sprintf("name:%q→%q", p.Name, fields.Name))
	}
	if fields.Description != "" && fields.Description != p.Description {
		updates["description"] = fields.Description
		changes = append(changes, "description updated")
	}
	if fields.Color != "" && fields.Color != p.Color {
		updates["color"] = fields.Color
		changes = append(changes, fmt.Sprintf("color:%s→%s", p.Color, fields.Color))
	}
	if err := s.db.Model(p).Updates(updates).Error; err != nil {
		return changes, fmt.Errorf("update project: %w", err)
	}
	return changes, nil
}

func (s *ProjectService) Delete(p *models.Project) error {
	return s.db.Delete(p).Error
}
