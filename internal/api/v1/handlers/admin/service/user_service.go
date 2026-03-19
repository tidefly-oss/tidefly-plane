package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin/helpers"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{db: db}
}

func (s *UserService) List() ([]models.User, error) {
	var users []models.User
	if err := s.db.Preload("ProjectMembers").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

func (s *UserService) GetByID(id string) (models.User, error) {
	var u models.User
	if err := s.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, fmt.Errorf("user not found: %w", err)
	}
	return u, nil
}

func (s *UserService) Create(email, name string, role models.UserRole) (models.User, string, error) {
	plain, hash, err := helpers.GenerateTempPassword()
	if err != nil {
		return models.User{}, "", fmt.Errorf("generate password: %w", err)
	}
	u := models.User{
		Email:               email,
		Name:                name,
		Role:                role,
		Password:            hash,
		Active:              true,
		ForcePasswordChange: true,
	}
	if err := s.db.Create(&u).Error; err != nil {
		return models.User{}, "", fmt.Errorf("create user: %w", err)
	}
	return u, plain, nil
}

func (s *UserService) Update(id string, name *string, role *models.UserRole, active *bool) (models.User, []string, error) {
	var u models.User
	if err := s.db.Preload("ProjectMembers").First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, nil, fmt.Errorf("user not found: %w", err)
	}

	var changes []string
	if name != nil && *name != u.Name {
		changes = append(changes, fmt.Sprintf("name:%q→%q", u.Name, *name))
		u.Name = *name
	}
	if role != nil && *role != u.Role {
		changes = append(changes, fmt.Sprintf("role:%s→%s", u.Role, *role))
		u.Role = *role
	}
	if active != nil && *active != u.Active {
		changes = append(changes, fmt.Sprintf("active:%v→%v", u.Active, *active))
		u.Active = *active
	}

	if err := s.db.Save(&u).Error; err != nil {
		return models.User{}, changes, fmt.Errorf("update user: %w", err)
	}
	return u, changes, nil
}

func (s *UserService) Delete(id string) (models.User, error) {
	var u models.User
	if err := s.db.First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, fmt.Errorf("user not found: %w", err)
	}
	if err := s.db.Delete(&models.User{}, "id = ?", id).Error; err != nil {
		return u, fmt.Errorf("delete user: %w", err)
	}
	return u, nil
}

func (s *UserService) ResetPassword(id string) (models.User, string, error) {
	var u models.User
	if err := s.db.First(&u, "id = ?", id).Error; err != nil {
		return models.User{}, "", fmt.Errorf("user not found: %w", err)
	}
	plain, hash, err := helpers.GenerateTempPassword()
	if err != nil {
		return models.User{}, "", fmt.Errorf("generate password: %w", err)
	}
	if err := s.db.Exec(
		"UPDATE users SET password = ?, force_password_change = true WHERE id = ?", hash, id,
	).Error; err != nil {
		return u, "", fmt.Errorf("reset password: %w", err)
	}
	return u, plain, nil
}

func (s *UserService) SetProjectMembers(userID string, projectIDs []string) (models.User, error) {
	var u models.User
	if err := s.db.First(&u, "id = ?", userID).Error; err != nil {
		return models.User{}, fmt.Errorf("user not found: %w", err)
	}

	if len(projectIDs) > 0 {
		var count int64
		s.db.Model(&models.Project{}).Where("id IN ?", projectIDs).Count(&count)
		if int(count) != len(projectIDs) {
			return models.User{}, fmt.Errorf("invalid project ids")
		}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
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
	if err != nil {
		return models.User{}, fmt.Errorf("set project members: %w", err)
	}

	s.db.Preload("ProjectMembers").First(&u, "id = ?", userID)
	return u, nil
}
