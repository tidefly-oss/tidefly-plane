package admin

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/auth"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// ── UserService ───────────────────────────────────────────────────────────────

type UserService struct {
	store *Store
}

func NewUserService(store *Store) *UserService {
	return &UserService{store: store}
}

func (s *UserService) List() ([]models.User, error) {
	return s.store.ListUsers()
}

func (s *UserService) GetByID(id string) (models.User, error) {
	return s.store.FindUserByID(id)
}

func (s *UserService) Create(email, name string, role models.UserRole) (models.User, string, error) {
	plain, hash, err := generateTempPassword()
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
	if err := s.store.CreateUser(&u); err != nil {
		return models.User{}, "", err
	}
	return u, plain, nil
}

func (s *UserService) Update(id string, name *string, role *models.UserRole, active *bool) (models.User, []string, error) {
	u, err := s.store.FindUserByID(id)
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
	if err := s.store.SaveUser(&u); err != nil {
		return models.User{}, changes, err
	}
	return u, changes, nil
}

func (s *UserService) Delete(id string) (models.User, error) {
	u, err := s.store.FindUserByID(id)
	if err != nil {
		return models.User{}, err
	}
	if err := s.store.DeleteUser(id); err != nil {
		return u, err
	}
	return u, nil
}

func (s *UserService) ResetPassword(id string) (models.User, string, error) {
	u, err := s.store.FindUserByID(id)
	if err != nil {
		return models.User{}, "", err
	}
	plain, hash, err := generateTempPassword()
	if err != nil {
		return models.User{}, "", fmt.Errorf("generate password: %w", err)
	}
	if err := s.store.ResetUserPassword(&u, hash); err != nil {
		return u, "", fmt.Errorf("reset password: %w", err)
	}
	return u, plain, nil
}

func (s *UserService) SetProjectMembers(userID string, projectIDs []string) (models.User, error) {
	if _, err := s.store.FindUserByID(userID); err != nil {
		return models.User{}, err
	}
	if len(projectIDs) > 0 {
		valid, err := s.store.ValidateProjectIDs(projectIDs)
		if err != nil {
			return models.User{}, err
		}
		if !valid {
			return models.User{}, fmt.Errorf("invalid project ids")
		}
	}
	if err := s.store.SetProjectMembers(userID, projectIDs); err != nil {
		return models.User{}, fmt.Errorf("set project members: %w", err)
	}
	return s.store.FindUserByID(userID)
}

// ── SettingsService ───────────────────────────────────────────────────────────

type SettingsUpdateInput struct {
	InstanceName                 *string
	InstanceURL                  *string
	RegistrationMode             *string
	CaddyBaseDomain              *string
	SMTPHost                     *string
	SMTPPort                     *int
	SMTPUsername                 *string
	SMTPPassword                 *string
	SMTPFrom                     *string
	SMTPTLSEnabled               *bool
	NotificationsEnabled         *bool
	ExternalNotificationsEnabled *bool
	SlackWebhookURL              *string
	DiscordWebhookURL            *string
	NotifyOnDeploy               *bool
	NotifyOnContainerDown        *bool
	NotifyOnWebhookFail          *bool
	APIDocsEnabled               *bool
}

type SettingsService struct {
	store *Store
	caddy *caddysvc.Client
}

func NewSettingsService(store *Store, caddy *caddysvc.Client) *SettingsService {
	return &SettingsService{store: store, caddy: caddy}
}

func (s *SettingsService) Get() (models.SystemSettings, error) {
	return s.store.GetSettings()
}

func (s *SettingsService) Update(input SettingsUpdateInput) (models.SystemSettings, error) {
	var settings models.SystemSettings
	if err := s.store.FirstOrCreateSettings(&settings); err != nil {
		return models.SystemSettings{}, err
	}

	applyIfSet(&settings.InstanceName, input.InstanceName)
	applyIfSet(&settings.InstanceURL, input.InstanceURL)
	applyIfSet(&settings.RegistrationMode, input.RegistrationMode)
	applyIfSet(&settings.CaddyBaseDomain, input.CaddyBaseDomain)
	applyIfSet(&settings.SMTPHost, input.SMTPHost)
	applyIfSet(&settings.SMTPPort, input.SMTPPort)
	applyIfSet(&settings.SMTPUsername, input.SMTPUsername)
	applyIfSet(&settings.SMTPPassword, input.SMTPPassword)
	applyIfSet(&settings.SMTPFrom, input.SMTPFrom)
	applyIfSet(&settings.SMTPTLSEnabled, input.SMTPTLSEnabled)
	applyIfSet(&settings.NotificationsEnabled, input.NotificationsEnabled)
	applyIfSet(&settings.ExternalNotificationsEnabled, input.ExternalNotificationsEnabled)
	applyIfSet(&settings.SlackWebhookURL, input.SlackWebhookURL)
	applyIfSet(&settings.DiscordWebhookURL, input.DiscordWebhookURL)
	applyIfSet(&settings.NotifyOnDeploy, input.NotifyOnDeploy)
	applyIfSet(&settings.NotifyOnContainerDown, input.NotifyOnContainerDown)
	applyIfSet(&settings.NotifyOnWebhookFail, input.NotifyOnWebhookFail)
	applyIfSet(&settings.APIDocsEnabled, input.APIDocsEnabled)

	if err := s.store.SaveSettings(&settings); err != nil {
		return models.SystemSettings{}, fmt.Errorf("update settings: %w", err)
	}
	if input.CaddyBaseDomain != nil && s.caddy != nil {
		s.caddy.SetBaseDomain(*input.CaddyBaseDomain)
	}
	return settings, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

func applyIfSet[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func generateTempPassword() (plain string, hash string, err error) {
	id := uuid.New().String()
	plain = id[:8] + id[9:13]
	hash, err = auth.HashPassword(plain)
	if err != nil {
		return "", "", err
	}
	return plain, hash, nil
}
