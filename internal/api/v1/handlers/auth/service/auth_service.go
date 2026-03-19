package service

import (
	"fmt"

	"github.com/aarondl/authboss/v3"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type AuthService struct {
	db *gorm.DB
	ab *authboss.Authboss
}

func New(db *gorm.DB, ab *authboss.Authboss) *AuthService {
	return &AuthService{db: db, ab: ab}
}

func (s *AuthService) GetFreshUser(id string) (models.User, error) {
	var u models.User
	if err := s.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (s *AuthService) ChangePassword(user *models.User, currentPassword, newPassword string) error {
	if err := s.ab.Core.Hasher.CompareHashAndPassword(user.Password, currentPassword); err != nil {
		return fmt.Errorf("wrong_current_password")
	}
	hash, err := s.ab.Core.Hasher.GenerateHash(newPassword)
	if err != nil {
		return fmt.Errorf("hash_failed")
	}
	if err := s.db.Exec(
		"UPDATE users SET password = ?, force_password_change = false WHERE id = ?",
		hash, user.ID,
	).Error; err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}
