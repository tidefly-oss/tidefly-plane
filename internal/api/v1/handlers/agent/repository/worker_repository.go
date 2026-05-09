package repository

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type WorkerRepository struct {
	db *gorm.DB
}

func NewWorkerRepository(db *gorm.DB) *WorkerRepository {
	return &WorkerRepository{db: db}
}

func (r *WorkerRepository) ExistsByID(id string) bool {
	var w models.WorkerNode
	return r.db.Where("id = ?", id).First(&w).Error == nil
}

func (r *WorkerRepository) Create(w *models.WorkerNode) error {
	if err := r.db.Create(w).Error; err != nil {
		return fmt.Errorf("create worker: %w", err)
	}
	return nil
}

func (r *WorkerRepository) FindActive(id string) (*models.WorkerNode, error) {
	var w models.WorkerNode
	if err := r.db.Where("id = ? AND status != ?", id, models.WorkerStatusRevoked).
		First(&w).Error; err != nil {
		return nil, fmt.Errorf("worker not found: %w", err)
	}
	return &w, nil
}

func (r *WorkerRepository) FindRevoked(id string) (*models.WorkerNode, error) {
	var w models.WorkerNode
	if err := r.db.Where("id = ? AND status = ?", id, models.WorkerStatusRevoked).
		First(&w).Error; err != nil {
		return nil, fmt.Errorf("worker not found or not revoked: %w", err)
	}
	return &w, nil
}

func (r *WorkerRepository) List() ([]models.WorkerNode, error) {
	var workers []models.WorkerNode
	if err := r.db.Order("created_at DESC").Find(&workers).Error; err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	return workers, nil
}

func (r *WorkerRepository) Revoke(id string) error {
	return r.db.Model(&models.WorkerNode{}).
		Where("id = ?", id).
		Update("status", models.WorkerStatusRevoked).Error
}

func (r *WorkerRepository) Delete(w *models.WorkerNode) error {
	return r.db.Delete(w).Error
}
