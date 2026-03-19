package http

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin/mapper"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type ListUsersInput struct{}
type ListUsersOutput struct {
	Body struct {
		Users []mapper.AdminUserResponse `json:"users"`
	}
}

type GetUserInput struct {
	ID string `path:"id" doc:"User ID"`
}
type GetUserOutput struct {
	Body mapper.AdminUserResponse
}

type CreateUserInput struct {
	Body struct {
		Email string          `json:"email" format:"email"`
		Name  string          `json:"name" minLength:"1"`
		Role  models.UserRole `json:"role" enum:"admin,member"`
	}
}
type CreateUserOutput struct {
	Body struct {
		User         mapper.AdminUserResponse `json:"user"`
		TempPassword string                   `json:"temp_password"`
	}
}

type UpdateUserInput struct {
	ID   string `path:"id"`
	Body struct {
		Name   *string          `json:"name,omitempty"`
		Role   *models.UserRole `json:"role,omitempty"`
		Active *bool            `json:"active,omitempty"`
	}
}
type UpdateUserOutput struct {
	Body mapper.AdminUserResponse
}

type DeleteUserInput struct {
	ID string `path:"id"`
}

func (h *Handler) ListUsers(_ context.Context, _ *ListUsersInput) (*ListUsersOutput, error) {
	users, err := h.users.List()
	if err != nil {
		return nil, err
	}
	out := &ListUsersOutput{}
	out.Body.Users = mapper.ToAdminUserResponses(users)
	return out, nil
}

func (h *Handler) GetUser(_ context.Context, input *GetUserInput) (*GetUserOutput, error) {
	u, err := h.users.GetByID(input.ID)
	if err != nil {
		return nil, huma404("user not found")
	}
	return &GetUserOutput{Body: mapper.ToAdminUserResponse(&u)}, nil
}

func (h *Handler) CreateUser(ctx context.Context, input *CreateUserInput) (*CreateUserOutput, error) {
	if input.Body.Role == "" {
		input.Body.Role = models.RoleMember
	}
	u, plain, err := h.users.Create(input.Body.Email, input.Body.Name, input.Body.Role)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditAdminUserCreate,
		ResourceID: u.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("email=%s role=%s", input.Body.Email, input.Body.Role),
	})
	if err != nil {
		return nil, huma409("email already exists")
	}
	out := &CreateUserOutput{}
	out.Body.User = mapper.ToAdminUserResponse(&u)
	out.Body.TempPassword = plain
	return out, nil
}

func (h *Handler) UpdateUser(ctx context.Context, input *UpdateUserInput) (*UpdateUserOutput, error) {
	u, changes, err := h.users.Update(input.ID, input.Body.Name, input.Body.Role, input.Body.Active)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditAdminUserUpdate,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("%v", changes),
	})
	if err != nil {
		return nil, err
	}
	return &UpdateUserOutput{Body: mapper.ToAdminUserResponse(&u)}, nil
}

func (h *Handler) DeleteUser(ctx context.Context, input *DeleteUserInput) (*struct{}, error) {
	u, err := h.users.Delete(input.ID)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditAdminUserDelete,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("email=%s", u.Email),
	})
	if err != nil {
		return nil, huma404("user not found")
	}
	return nil, nil
}
