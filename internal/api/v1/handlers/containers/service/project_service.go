package service

import (
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/containers/repository"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

type ProjectService struct {
	repo *repository.ProjectRepository
}

func NewProjectService(db *gorm.DB) *ProjectService {
	return &ProjectService{repo: repository.NewProjectRepository(db)}
}

func (s *ProjectService) GetByID(id string) (models.Project, error) {
	return s.repo.GetByID(id)
}
