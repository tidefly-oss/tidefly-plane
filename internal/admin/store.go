package admin

import (
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

// ── Users ─────────────────────────────────────────────────────────────────────

func (s *Store) ListUsers() ([]models.User, error) {
	var users []models.User
	if err := s.db.Preload("ProjectMembers").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

func (s *Store) FindUserByID(id string) (models.User, error) {
	var u models.User
	if err := s.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (s *Store) CreateUser(u *models.User) error {
	if err := s.db.Create(u).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *Store) SaveUser(u *models.User) error {
	if err := s.db.Save(u).Error; err != nil {
		return fmt.Errorf("save user: %w", err)
	}
	return nil
}

func (s *Store) DeleteUser(id string) error {
	if err := s.db.Delete(&models.User{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (s *Store) ResetUserPassword(u *models.User, hash string) error {
	return s.db.Model(u).Updates(map[string]any{
		"password":              hash,
		"force_password_change": true,
	}).Error
}

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

// ── Settings ──────────────────────────────────────────────────────────────────

func (s *Store) GetSettings() (models.SystemSettings, error) {
	var settings models.SystemSettings
	if err := s.db.First(&settings).Error; err != nil {
		return models.SystemSettings{}, nil
	}
	return settings, nil
}

func (s *Store) SaveSettings(settings *models.SystemSettings) error {
	if err := s.db.Session(&gorm.Session{FullSaveAssociations: false}).
		Select("*").Save(settings).Error; err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

func (s *Store) FirstOrCreateSettings(settings *models.SystemSettings) error {
	if err := s.db.FirstOrCreate(settings).Error; err != nil {
		return fmt.Errorf("first or create settings: %w", err)
	}
	return nil
}
