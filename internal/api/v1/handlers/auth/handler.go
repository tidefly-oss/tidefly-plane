package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aarondl/authboss/v3"
	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type Handler struct {
	ab  *authboss.Authboss
	db  *gorm.DB
	log *logger.Logger
}

func New(ab *authboss.Authboss, db *gorm.DB, log *logger.Logger) *Handler {
	return &Handler{ab: ab, db: db, log: log}
}

// ── CurrentUser ───────────────────────────────────────────────────────────────

type CurrentUserInput struct{}

type CurrentUserOutput struct {
	Body struct {
		User struct {
			ID                  string          `json:"id"`
			Email               string          `json:"email"`
			Name                string          `json:"name"`
			Role                models.UserRole `json:"role"`
			ForcePasswordChange bool            `json:"force_password_change"`
			ProjectIDs          []string        `json:"project_ids"`
		} `json:"user"`
	}
}

func (h *Handler) CurrentUser(ctx context.Context, _ *CurrentUserInput) (*CurrentUserOutput, error) {
	u := middleware.UserFromHumaCtx(ctx)
	if u == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	abUser, ok := u.(*models.User)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	var fresh models.User
	if err := h.db.Preload("ProjectMembers").First(&fresh, "id = ?", abUser.ID).Error; err != nil {
		return nil, huma.Error401Unauthorized("user not found")
	}

	projectIDs := make([]string, 0, len(fresh.ProjectMembers))
	for _, pm := range fresh.ProjectMembers {
		projectIDs = append(projectIDs, pm.ProjectID)
	}

	out := &CurrentUserOutput{}
	out.Body.User.ID = fresh.ID
	out.Body.User.Email = fresh.Email
	out.Body.User.Name = fresh.Name
	out.Body.User.Role = fresh.Role
	out.Body.User.ForcePasswordChange = fresh.ForcePasswordChange
	out.Body.User.ProjectIDs = projectIDs
	return out, nil
}

// ── ChangePassword ────────────────────────────────────────────────────────────

type ChangePasswordInput struct {
	Body struct {
		CurrentPassword string `json:"current_password" minLength:"1" doc:"Current password"`
		NewPassword     string `json:"new_password" minLength:"8" doc:"New password (min 8 chars)"`
	}
}

type ChangePasswordOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (h *Handler) ChangePassword(ctx context.Context, input *ChangePasswordInput) (*ChangePasswordOutput, error) {
	u := middleware.UserFromHumaCtx(ctx)
	if u == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	abUser, ok := u.(*models.User)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	if err := h.ab.Config.Core.Hasher.CompareHashAndPassword(abUser.Password, input.Body.CurrentPassword); err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditPasswordChange, ResourceID: abUser.ID, Success: false,
				Details: fmt.Sprintf("email=%s reason=wrong_current_password", abUser.Email),
			},
		)
		return nil, huma.NewError(http.StatusUnauthorized, "current password is incorrect")
	}

	hash, err := h.ab.Config.Core.Hasher.GenerateHash(input.Body.NewPassword)
	if err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditPasswordChange, ResourceID: abUser.ID, Success: false,
				Details: fmt.Sprintf("email=%s reason=hash_failed", abUser.Email),
			},
		)
		return nil, fmt.Errorf("hash password: %w", err)
	}

	err = h.db.Exec(
		"UPDATE users SET password = ?, force_password_change = false WHERE id = ?",
		hash, abUser.ID,
	).Error
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditPasswordChange, ResourceID: abUser.ID, Success: err == nil,
			Details: fmt.Sprintf("email=%s", abUser.Email),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("update password: %w", err)
	}

	out := &ChangePasswordOutput{}
	out.Body.Message = "password changed successfully"
	return out, nil
}
