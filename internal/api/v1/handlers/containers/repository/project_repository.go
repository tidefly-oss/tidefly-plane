package repository

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) GetByID(id string) (models.Project, error) {
	var p models.Project
	if err := r.db.First(&p, "id = ?", id).Error; err != nil {
		return models.Project{}, fmt.Errorf("project not found: %w", err)
	}
	return p, nil
}
