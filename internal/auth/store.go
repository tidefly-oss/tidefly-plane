package auth

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// ── Queries ───────────────────────────────────────────────────────────────────

func (s *Store) FindByEmail(email string) (*models.User, error) {
	var u models.User
	if err := s.db.Where("email = ?", email).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("find by email: %w", err)
	}
	return &u, nil
}

func (s *Store) FindByID(id string) (*models.User, error) {
	var u models.User
	if err := s.db.First(&u, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	return &u, nil
}

func (s *Store) FindByIDWithProjects(id string) (*models.User, error) {
	var u models.User
	if err := s.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	return &u, nil
}

func (s *Store) List() ([]models.User, error) {
	var users []models.User
	if err := s.db.Preload("ProjectMembers").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

func (s *Store) Count() (int64, error) {
	var count int64
	if err := s.db.Model(&models.User{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ── Writes ────────────────────────────────────────────────────────────────────

func (s *Store) Create(u *models.User) error {
	if err := s.db.Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *Store) Save(u *models.User) error {
	if err := s.db.Save(u).Error; err != nil {
		return fmt.Errorf("save user: %w", err)
	}
	return nil
}

func (s *Store) Delete(id string) error {
	if err := s.db.Delete(&models.User{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (s *Store) UpdatePassword(u *models.User, hash string, forceChange bool) error {
	return s.db.Model(u).Updates(map[string]any{
		"password":              hash,
		"force_password_change": forceChange,
	}).Error
}

// ── Project membership ────────────────────────────────────────────────────────

func (s *Store) ValidateProjectIDs(projectIDs []string) (bool, error) {
	var count int64
	if err := s.db.Model(&models.Project{}).Where("id IN ?", projectIDs).Count(&count).Error; err != nil {
		return false, err
	}
	return int(count) == len(projectIDs), nil
}

func (s *Store) SetProjectMembers(userID string, projectIDs []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
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
