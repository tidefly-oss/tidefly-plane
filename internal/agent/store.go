package agent

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

func (s *Store) ExistsByID(id string) bool {
	var w models.WorkerNode
	return s.db.Where("id = ?", id).First(&w).Error == nil
}

func (s *Store) Create(w *models.WorkerNode) error {
	if err := s.db.Create(w).Error; err != nil {
		return fmt.Errorf("create worker: %w", err)
	}
	return nil
}

func (s *Store) FindActive(id string) (*models.WorkerNode, error) {
	var w models.WorkerNode
	if err := s.db.Where("id = ? AND status != ?", id, models.WorkerStatusRevoked).
		First(&w).Error; err != nil {
		return nil, fmt.Errorf("worker not found: %w", err)
	}
	return &w, nil
}

func (s *Store) FindRevoked(id string) (*models.WorkerNode, error) {
	var w models.WorkerNode
	if err := s.db.Where("id = ? AND status = ?", id, models.WorkerStatusRevoked).
		First(&w).Error; err != nil {
		return nil, fmt.Errorf("worker not found or not revoked: %w", err)
	}
	return &w, nil
}

func (s *Store) List() ([]models.WorkerNode, error) {
	var workers []models.WorkerNode
	if err := s.db.Order("created_at DESC").Find(&workers).Error; err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	return workers, nil
}

func (s *Store) Revoke(id string) error {
	return s.db.Model(&models.WorkerNode{}).
		Where("id = ?", id).
		Update("status", models.WorkerStatusRevoked).Error
}

func (s *Store) Delete(w *models.WorkerNode) error {
	return s.db.Delete(w).Error
}
