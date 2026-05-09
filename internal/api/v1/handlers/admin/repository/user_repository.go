package repository

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) List() ([]models.User, error) {
	var users []models.User
	if err := r.db.Preload("ProjectMembers").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

func (r *UserRepository) GetByID(id string) (models.User, error) {
	var u models.User
	if err := r.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (r *UserRepository) Create(u *models.User) error {
	if err := r.db.Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) Save(u *models.User) error {
	if err := r.db.Save(u).Error; err != nil {
		return fmt.Errorf("save user: %w", err)
	}
	return nil
}

func (r *UserRepository) Delete(id string) error {
	if err := r.db.Delete(&models.User{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (r *UserRepository) ResetPassword(u *models.User, hash string) error {
	return r.db.Model(u).Updates(map[string]any{
		"password":              hash,
		"force_password_change": true,
	}).Error
}

func (r *UserRepository) ValidateProjectIDs(projectIDs []string) (bool, error) {
	var count int64
	if err := r.db.Model(&models.Project{}).Where("id IN ?", projectIDs).Count(&count).Error; err != nil {
		return false, err
	}
	return int(count) == len(projectIDs), nil
}

func (r *UserRepository) SetProjectMembers(userID string, projectIDs []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.ProjectMember{}, "user_id = ?", userID).Error; err != nil {
			return err
		}
		for _, pid := range projectIDs {
			pm := models.ProjectMember{
				ID:        uuid.New().String(),
				UserID:    userID,
				ProjectID: pid,
				Role:      models.RoleMember,
			}
			if err := tx.Create(&pm).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
