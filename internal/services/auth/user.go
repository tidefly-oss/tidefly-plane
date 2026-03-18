package auth

import (
	"context"
	"errors"

	"github.com/aarondl/authboss/v3"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

// Storer implements authboss.ServerStorer using models.User from internal/models.

type Storer struct {
	db *gorm.DB
}

func NewStorer(db *gorm.DB) *Storer {
	return &Storer{db: db}
}

func (s *Storer) New(_ context.Context) authboss.User {
	return &models.User{}
}

func (s *Storer) Create(_ context.Context, user authboss.User) error {
	u := user.(*models.User)
	return s.db.Create(u).Error
}

func (s *Storer) Load(_ context.Context, key string) (authboss.User, error) {
	if key == "" {
		return nil, authboss.ErrUserNotFound
	}
	var u models.User
	if err := s.db.Where("email = ?", key).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, authboss.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *Storer) Save(_ context.Context, user authboss.User) error {
	u := user.(*models.User)
	return s.db.Model(u).Where("id = ?", u.ID).Updates(
		map[string]interface{}{
			"email":                u.Email,
			"password":             u.Password,
			"attempt_count":        u.AttemptCount,
			"last_attempt":         u.LastAttempt,
			"locked":               u.Locked,
			"recover_selector":     u.RecoverSelector,
			"recover_verifier":     u.RecoverVerifier,
			"recover_token_expiry": u.RecoverTokenExpiry,
			"confirm_selector":     u.ConfirmSelector,
			"confirm_verifier":     u.ConfirmVerifier,
			"confirmed":            u.Confirmed,
		},
	).Error
}

func (s *Storer) AddRememberToken(_ context.Context, pid, token string) error {
	var u models.User
	if err := s.db.Where("email = ?", pid).First(&u).Error; err != nil {
		return err
	}
	return s.db.Create(&models.Token{UserID: u.ID, Token: token}).Error
}

func (s *Storer) DelRememberTokens(_ context.Context, pid string) error {
	var u models.User
	if err := s.db.Where("email = ?", pid).First(&u).Error; err != nil {
		return err
	}
	return s.db.Where("user_id = ?", u.ID).Delete(&models.Token{}).Error
}

func (s *Storer) UseRememberToken(_ context.Context, pid, token string) error {
	var u models.User
	if err := s.db.Where("email = ?", pid).First(&u).Error; err != nil {
		return err
	}
	result := s.db.Where("user_id = ? AND token = ?", u.ID, token).Delete(&models.Token{})
	if result.RowsAffected == 0 {
		return authboss.ErrTokenNotFound
	}
	return result.Error
}

func (s *Storer) LoadByRecoverSelector(_ context.Context, selector string) (authboss.RecoverableUser, error) {
	var u models.User
	if err := s.db.Where("recover_selector = ?", selector).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, authboss.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *Storer) LoadByConfirmSelector(_ context.Context, selector string) (authboss.ConfirmableUser, error) {
	var u models.User
	if err := s.db.Where("confirm_selector = ?", selector).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, authboss.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}
