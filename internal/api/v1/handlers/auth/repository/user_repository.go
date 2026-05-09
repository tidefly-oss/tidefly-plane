package repository

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByEmail(email string) (*models.User, error) {
	var u models.User
	if err := r.db.Where("email = ?", email).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("find by email: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) FindByID(id string) (*models.User, error) {
	var u models.User
	if err := r.db.First(&u, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) FindByIDWithProjects(id string) (*models.User, error) {
	var u models.User
	if err := r.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) Count() (int64, error) {
	var count int64
	if err := r.db.Model(&models.User{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *UserRepository) Create(u *models.User) error {
	if err := r.db.Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdatePassword(user *models.User, hash string, forceChange bool) error {
	return r.db.Model(user).Updates(map[string]any{
		"password":              hash,
		"force_password_change": forceChange,
	}).Error
}
