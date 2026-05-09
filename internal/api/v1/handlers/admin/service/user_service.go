package service

import (
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/helpers"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/repository"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

type UserService struct {
	repo *repository.UserRepository
}

func NewUserService(db *gorm.DB) *UserService {
	return &UserService{repo: repository.NewUserRepository(db)}
}

func (s *UserService) List() ([]models.User, error) {
	return s.repo.List()
}

func (s *UserService) GetByID(id string) (models.User, error) {
	return s.repo.GetByID(id)
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
	if err := s.repo.Create(&u); err != nil {
		return models.User{}, "", err
	}
	return u, plain, nil
}

func (s *UserService) Update(id string, name *string, role *models.UserRole, active *bool) (models.User, []string, error) {
	u, err := s.repo.GetByID(id)
	if err != nil {
		return models.User{}, nil, err
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

	if err := s.repo.Save(&u); err != nil {
		return models.User{}, changes, err
	}
	return u, changes, nil
}

func (s *UserService) Delete(id string) (models.User, error) {
	u, err := s.repo.GetByID(id)
	if err != nil {
		return models.User{}, err
	}
	if err := s.repo.Delete(id); err != nil {
		return u, err
	}
	return u, nil
}

func (s *UserService) ResetPassword(id string) (models.User, string, error) {
	u, err := s.repo.GetByID(id)
	if err != nil {
		return models.User{}, "", err
	}
	plain, hash, err := helpers.GenerateTempPassword()
	if err != nil {
		return models.User{}, "", fmt.Errorf("generate password: %w", err)
	}
	if err := s.repo.ResetPassword(&u, hash); err != nil {
		return u, "", fmt.Errorf("reset password: %w", err)
	}
	return u, plain, nil
}

func (s *UserService) SetProjectMembers(userID string, projectIDs []string) (models.User, error) {
	if _, err := s.repo.GetByID(userID); err != nil {
		return models.User{}, err
	}
	if len(projectIDs) > 0 {
		valid, err := s.repo.ValidateProjectIDs(projectIDs)
		if err != nil {
			return models.User{}, err
		}
		if !valid {
			return models.User{}, fmt.Errorf("invalid project ids")
		}
	}
	if err := s.repo.SetProjectMembers(userID, projectIDs); err != nil {
		return models.User{}, fmt.Errorf("set project members: %w", err)
	}
	return s.repo.GetByID(userID)
}
