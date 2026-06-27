package admin

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

// ── ListUsers ─────────────────────────────────────────────────────────────────

type listUsersOutput struct {
	Body struct {
		Users []UserResponse `json:"users"`
	}
}

func (h *Handler) listUsers(_ context.Context, _ *struct{}) (*listUsersOutput, error) {
	users, err := h.users.List()
	if err != nil {
		return nil, err
	}
	out := &listUsersOutput{}
	out.Body.Users = toUserResponses(users)
	return out, nil
}

// ── GetUser ───────────────────────────────────────────────────────────────────

type getUserInput struct {
	ID string `path:"id" doc:"User ID"`
}

type getUserOutput struct {
	Body UserResponse
}

func (h *Handler) getUser(_ context.Context, input *getUserInput) (*getUserOutput, error) {
	u, err := h.users.GetByID(input.ID)
	if err != nil {
		return nil, huma404("user not found")
	}
	return &getUserOutput{Body: toUserResponse(&u)}, nil
}

// ── CreateUser ────────────────────────────────────────────────────────────────

type createUserInput struct {
	Body struct {
		Email string          `json:"email" format:"email"`
		Name  string          `json:"name"  minLength:"1"`
		Role  models.UserRole `json:"role"  enum:"admin,member"`
	}
}

type createUserOutput struct {
	Body struct {
		User         UserResponse `json:"user"`
		TempPassword string       `json:"temp_password"`
	}
}

func (h *Handler) createUser(ctx context.Context, input *createUserInput) (*createUserOutput, error) {
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
	out := &createUserOutput{}
	out.Body.User = toUserResponse(&u)
	out.Body.TempPassword = plain
	return out, nil
}

// ── UpdateUser ────────────────────────────────────────────────────────────────

type updateUserInput struct {
	ID   string `path:"id"`
	Body struct {
		Name   *string          `json:"name,omitempty"`
		Role   *models.UserRole `json:"role,omitempty"`
		Active *bool            `json:"active,omitempty"`
	}
}

type updateUserOutput struct {
	Body UserResponse
}

func (h *Handler) updateUser(ctx context.Context, input *updateUserInput) (*updateUserOutput, error) {
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
	return &updateUserOutput{Body: toUserResponse(&u)}, nil
}

// ── DeleteUser ────────────────────────────────────────────────────────────────

type deleteUserInput struct {
	ID string `path:"id"`
}

func (h *Handler) deleteUser(ctx context.Context, input *deleteUserInput) (*struct{}, error) {
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

// ── ResetUserPassword ─────────────────────────────────────────────────────────

type resetUserPasswordInput struct {
	ID string `path:"id" doc:"User ID"`
}

type resetUserPasswordOutput struct {
	Body struct {
		TempPassword string `json:"temp_password"`
	}
}

func (h *Handler) resetUserPassword(ctx context.Context, input *resetUserPasswordInput) (*resetUserPasswordOutput, error) {
	u, plain, err := h.users.ResetPassword(input.ID)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditAdminUserPasswordReset,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("email=%s", u.Email),
	})
	if err != nil {
		return nil, huma404("user not found")
	}
	out := &resetUserPasswordOutput{}
	out.Body.TempPassword = plain
	return out, nil
}

// ── SetProjectMembers ─────────────────────────────────────────────────────────

type setProjectMembersInput struct {
	ID   string `path:"id" doc:"User ID"`
	Body struct {
		ProjectIDs []string `json:"project_ids"`
	}
}

type setProjectMembersOutput struct {
	Body UserResponse
}

func (h *Handler) setProjectMembers(ctx context.Context, input *setProjectMembersInput) (*setProjectMembersOutput, error) {
	if input.Body.ProjectIDs == nil {
		input.Body.ProjectIDs = []string{}
	}
	u, err := h.users.SetProjectMembers(input.ID, input.Body.ProjectIDs)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditAdminUserProjectsUpdate,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("email=%s projects=%d", u.Email, len(input.Body.ProjectIDs)),
	})
	if err != nil {
		if err.Error() == "invalid project ids" {
			return nil, huma400("one or more project IDs are invalid")
		}
		return nil, huma404("user not found")
	}
	return &setProjectMembersOutput{Body: toUserResponse(&u)}, nil
}
