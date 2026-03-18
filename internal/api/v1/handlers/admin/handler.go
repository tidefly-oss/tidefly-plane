package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type Handler struct {
	db       *gorm.DB
	log      *logger.Logger
	notifier *notifiersvc.Service
}

func New(db *gorm.DB, log *logger.Logger, notifier *notifiersvc.Service) *Handler {
	return &Handler{db: db, log: log, notifier: notifier}
}

// ── Response types ────────────────────────────────────────────────────────────

type adminUserResponse struct {
	ID                  string          `json:"id"`
	Email               string          `json:"email"`
	Name                string          `json:"name"`
	Role                models.UserRole `json:"role"`
	Active              bool            `json:"active"`
	ForcePasswordChange bool            `json:"force_password_change"`
	CreatedAt           time.Time       `json:"created_at"`
	ProjectIDs          []string        `json:"project_ids"`
}

func toAdminUserResponse(u models.User) adminUserResponse {
	ids := make([]string, 0, len(u.ProjectMembers))
	for _, pm := range u.ProjectMembers {
		ids = append(ids, pm.ProjectID)
	}
	return adminUserResponse{
		ID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role,
		Active: u.Active, ForcePasswordChange: u.ForcePasswordChange,
		CreatedAt: u.CreatedAt, ProjectIDs: ids,
	}
}

// ── ListUsers ─────────────────────────────────────────────────────────────────

type ListUsersInput struct{}
type ListUsersOutput struct {
	Body struct {
		Users []adminUserResponse `json:"users"`
	}
}

func (h *Handler) ListUsers(_ context.Context, _ *ListUsersInput) (*ListUsersOutput, error) {
	var users []models.User
	if err := h.db.Preload("ProjectMembers").Find(&users).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	out := &ListUsersOutput{}
	out.Body.Users = make([]adminUserResponse, len(users))
	for i, u := range users {
		out.Body.Users[i] = toAdminUserResponse(u)
	}
	return out, nil
}

// ── GetUser ───────────────────────────────────────────────────────────────────

type GetUserInput struct {
	ID string `path:"id" doc:"User ID"`
}
type GetUserOutput struct {
	Body adminUserResponse
}

func (h *Handler) GetUser(_ context.Context, input *GetUserInput) (*GetUserOutput, error) {
	var u models.User
	if err := h.db.Preload("ProjectMembers").First(&u, "id = ?", input.ID).Error; err != nil {
		return nil, huma404("user not found")
	}
	return &GetUserOutput{Body: toAdminUserResponse(u)}, nil
}

// ── CreateUser ────────────────────────────────────────────────────────────────

type CreateUserInput struct {
	Body struct {
		Email string          `json:"email" format:"email" doc:"User email"`
		Name  string          `json:"name" minLength:"1" doc:"Display name"`
		Role  models.UserRole `json:"role" enum:"admin,member" doc:"User role"`
	}
}
type CreateUserOutput struct {
	Body struct {
		User         adminUserResponse `json:"user"`
		TempPassword string            `json:"temp_password"`
	}
}

func (h *Handler) CreateUser(ctx context.Context, input *CreateUserInput) (*CreateUserOutput, error) {
	if input.Body.Role == "" {
		input.Body.Role = models.RoleMember
	}
	plain, hash, err := generateTempPassword()
	if err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}
	u := models.User{
		Email: input.Body.Email, Name: input.Body.Name, Role: input.Body.Role,
		Password: hash, Active: true, ForcePasswordChange: true,
	}
	if err := h.db.Create(&u).Error; err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditAdminUserCreate, Success: false,
				Details: fmt.Sprintf("email=%s role=%s err=%s", input.Body.Email, input.Body.Role, err),
			},
		)
		return nil, huma409("email already exists")
	}
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditAdminUserCreate, ResourceID: u.ID, Success: true,
			Details: fmt.Sprintf("email=%s role=%s", u.Email, u.Role),
		},
	)
	out := &CreateUserOutput{}
	out.Body.User = toAdminUserResponse(u)
	out.Body.TempPassword = plain
	return out, nil
}

// ── UpdateUser ────────────────────────────────────────────────────────────────

type UpdateUserInput struct {
	ID   string `path:"id" doc:"User ID"`
	Body struct {
		Name   *string          `json:"name,omitempty"`
		Role   *models.UserRole `json:"role,omitempty"`
		Active *bool            `json:"active,omitempty"`
	}
}
type UpdateUserOutput struct {
	Body adminUserResponse
}

func (h *Handler) UpdateUser(ctx context.Context, input *UpdateUserInput) (*UpdateUserOutput, error) {
	var u models.User
	if err := h.db.Preload("ProjectMembers").First(&u, "id = ?", input.ID).Error; err != nil {
		return nil, huma404("user not found")
	}
	var changes []string
	if input.Body.Name != nil && *input.Body.Name != u.Name {
		changes = append(changes, fmt.Sprintf("name:%q→%q", u.Name, *input.Body.Name))
		u.Name = *input.Body.Name
	}
	if input.Body.Role != nil && *input.Body.Role != u.Role {
		changes = append(changes, fmt.Sprintf("role:%s→%s", u.Role, *input.Body.Role))
		u.Role = *input.Body.Role
	}
	if input.Body.Active != nil && *input.Body.Active != u.Active {
		changes = append(changes, fmt.Sprintf("active:%v→%v", u.Active, *input.Body.Active))
		u.Active = *input.Body.Active
	}
	if err := h.db.Save(&u).Error; err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditAdminUserUpdate, ResourceID: u.ID, Success: false,
				Details: strings.Join(changes, " ") + " err=" + err.Error(),
			},
		)
		return nil, fmt.Errorf("update user: %w", err)
	}
	if len(changes) > 0 {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditAdminUserUpdate, ResourceID: u.ID, Success: true,
				Details: strings.Join(changes, " "),
			},
		)
	}
	return &UpdateUserOutput{Body: toAdminUserResponse(u)}, nil
}

// ── DeleteUser ────────────────────────────────────────────────────────────────

type DeleteUserInput struct {
	ID string `path:"id" doc:"User ID"`
}

func (h *Handler) DeleteUser(ctx context.Context, input *DeleteUserInput) (*struct{}, error) {
	var u models.User
	if err := h.db.First(&u, "id = ?", input.ID).Error; err != nil {
		return nil, huma404("user not found")
	}
	err := h.db.Delete(&models.User{}, "id = ?", input.ID).Error
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditAdminUserDelete, ResourceID: input.ID, Success: err == nil,
			Details: fmt.Sprintf("email=%s", u.Email),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("delete user: %w", err)
	}
	return nil, nil
}

// ── ResetUserPassword ─────────────────────────────────────────────────────────

type ResetUserPasswordInput struct {
	ID string `path:"id" doc:"User ID"`
}
type ResetUserPasswordOutput struct {
	Body struct {
		TempPassword string `json:"temp_password"`
	}
}

func (h *Handler) ResetUserPassword(ctx context.Context, input *ResetUserPasswordInput) (
	*ResetUserPasswordOutput, error,
) {
	var u models.User
	if err := h.db.First(&u, "id = ?", input.ID).Error; err != nil {
		return nil, huma404("user not found")
	}
	plain, hash, err := generateTempPassword()
	if err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}
	err = h.db.Exec("UPDATE users SET password = ?, force_password_change = true WHERE id = ?", hash, u.ID).Error
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditAdminUserPasswordReset, ResourceID: input.ID, Success: err == nil,
			Details: fmt.Sprintf("email=%s", u.Email),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("reset password: %w", err)
	}
	out := &ResetUserPasswordOutput{}
	out.Body.TempPassword = plain
	return out, nil
}

// ── SetProjectMembers ─────────────────────────────────────────────────────────

type SetProjectMembersInput struct {
	ID   string `path:"id" doc:"User ID"`
	Body struct {
		ProjectIDs []string `json:"project_ids" doc:"Project IDs to assign"`
	}
}
type SetProjectMembersOutput struct {
	Body adminUserResponse
}

func (h *Handler) SetProjectMembers(ctx context.Context, input *SetProjectMembersInput) (
	*SetProjectMembersOutput, error,
) {
	var u models.User
	if err := h.db.First(&u, "id = ?", input.ID).Error; err != nil {
		return nil, huma404("user not found")
	}
	if input.Body.ProjectIDs == nil {
		input.Body.ProjectIDs = []string{}
	}
	if len(input.Body.ProjectIDs) > 0 {
		var count int64
		h.db.Model(&models.Project{}).Where("id IN ?", input.Body.ProjectIDs).Count(&count)
		if int(count) != len(input.Body.ProjectIDs) {
			return nil, huma400("one or more project IDs are invalid")
		}
	}
	err := h.db.Transaction(
		func(tx *gorm.DB) error {
			if err := tx.Delete(&models.ProjectMember{}, "user_id = ?", input.ID).Error; err != nil {
				return err
			}
			for _, pid := range input.Body.ProjectIDs {
				pm := models.ProjectMember{
					ID: uuid.New().String(), UserID: input.ID, ProjectID: pid, Role: models.RoleMember,
				}
				if err := tx.Create(&pm).Error; err != nil {
					return err
				}
			}
			return nil
		},
	)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditAdminUserProjectsUpdate, ResourceID: input.ID, Success: err == nil,
			Details: fmt.Sprintf(
				"email=%s projects=%d [%s]", u.Email, len(input.Body.ProjectIDs),
				strings.Join(input.Body.ProjectIDs, ","),
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("set project members: %w", err)
	}
	h.db.Preload("ProjectMembers").First(&u, "id = ?", input.ID)
	return &SetProjectMembersOutput{Body: toAdminUserResponse(u)}, nil
}

// ── GetSettings ───────────────────────────────────────────────────────────────

type GetSettingsInput struct{}
type GetSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) GetSettings(_ context.Context, _ *GetSettingsInput) (*GetSettingsOutput, error) {
	var s models.SystemSettings
	if err := h.db.First(&s).Error; err != nil {
		return &GetSettingsOutput{Body: models.SystemSettings{}}, nil
	}
	return &GetSettingsOutput{Body: s}, nil
}

// ── UpdateSettings ────────────────────────────────────────────────────────────

type UpdateSettingsInput struct {
	Body struct {
		InstanceName          *string `json:"instance_name,omitempty"`
		InstanceURL           *string `json:"instance_url,omitempty"`
		RegistrationMode      *string `json:"registration_mode,omitempty"`
		SMTPHost              *string `json:"smtp_host,omitempty"`
		SMTPPort              *int    `json:"smtp_port,omitempty"`
		SMTPUsername          *string `json:"smtp_username,omitempty"`
		SMTPPassword          *string `json:"smtp_password,omitempty"`
		SMTPFrom              *string `json:"smtp_from,omitempty"`
		SMTPTLSEnabled        *bool   `json:"smtp_tls_enabled,omitempty"`
		SessionTimeoutHours   *int    `json:"session_timeout_hours,omitempty"`
		NotificationsEnabled  *bool   `json:"notifications_enabled,omitempty"`
		SlackWebhookURL       *string `json:"slack_webhook_url,omitempty"`
		DiscordWebhookURL     *string `json:"discord_webhook_url,omitempty"`
		NotifyOnDeploy        *bool   `json:"notify_on_deploy,omitempty"`
		NotifyOnContainerDown *bool   `json:"notify_on_container_down,omitempty"`
		NotifyOnWebhookFail   *bool   `json:"notify_on_webhook_fail,omitempty"`
	}
}
type UpdateSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) UpdateSettings(ctx context.Context, input *UpdateSettingsInput) (*UpdateSettingsOutput, error) {
	var s models.SystemSettings
	h.db.FirstOrCreate(&s)
	if input.Body.InstanceName != nil {
		s.InstanceName = *input.Body.InstanceName
	}
	if input.Body.InstanceURL != nil {
		s.InstanceURL = *input.Body.InstanceURL
	}
	if input.Body.RegistrationMode != nil {
		s.RegistrationMode = *input.Body.RegistrationMode
	}
	if input.Body.SMTPHost != nil {
		s.SMTPHost = *input.Body.SMTPHost
	}
	if input.Body.SMTPPort != nil {
		s.SMTPPort = *input.Body.SMTPPort
	}
	if input.Body.SMTPUsername != nil {
		s.SMTPUsername = *input.Body.SMTPUsername
	}
	if input.Body.SMTPPassword != nil {
		s.SMTPPassword = *input.Body.SMTPPassword
	}
	if input.Body.NotificationsEnabled != nil {
		s.NotificationsEnabled = *input.Body.NotificationsEnabled
	}
	if input.Body.SlackWebhookURL != nil {
		s.SlackWebhookURL = *input.Body.SlackWebhookURL
	}
	if input.Body.DiscordWebhookURL != nil {
		s.DiscordWebhookURL = *input.Body.DiscordWebhookURL
	}
	if input.Body.NotifyOnDeploy != nil {
		s.NotifyOnDeploy = *input.Body.NotifyOnDeploy
	}
	if input.Body.NotifyOnContainerDown != nil {
		s.NotifyOnContainerDown = *input.Body.NotifyOnContainerDown
	}
	if input.Body.NotifyOnWebhookFail != nil {
		s.NotifyOnWebhookFail = *input.Body.NotifyOnWebhookFail
	}
	err := h.db.Save(&s).Error
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditAdminSettingsUpdate, Success: err == nil,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("update settings: %w", err)
	}
	return &UpdateSettingsOutput{Body: s}, nil
}

type TestNotificationInput struct {
	Channel string `path:"channel" doc:"Channel to test: slack, discord, email"`
}

func (h *Handler) TestNotification(ctx context.Context, input *TestNotificationInput) (*struct{}, error) {
	if err := h.notifier.Test(ctx, input.Channel); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return nil, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func generateTempPassword() (plain string, hash string, err error) {
	id := uuid.New().String()
	plain = id[:8] + id[9:13]
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return plain, string(b), nil
}

// huma error helpers — kürzer als huma.NewError(...)
func huma400(msg string) error { return huma.Error400BadRequest(msg) }
func huma404(msg string) error { return huma.Error404NotFound(msg) }
func huma409(msg string) error { return huma.Error409Conflict(msg) }
