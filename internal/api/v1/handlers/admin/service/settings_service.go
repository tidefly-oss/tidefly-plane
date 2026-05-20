package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/helpers"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/repository"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type SettingsUpdateInput struct {
	InstanceName          *string
	InstanceURL           *string
	RegistrationMode      *string
	CaddyBaseDomain       *string
	SMTPHost              *string
	SMTPPort              *int
	SMTPUsername          *string
	SMTPPassword          *string
	SMTPFrom              *string
	SMTPTLSEnabled        *bool
	SessionTimeoutHours   *int
	NotificationsEnabled  *bool
	SlackWebhookURL       *string
	DiscordWebhookURL     *string
	NotifyOnDeploy        *bool
	NotifyOnContainerDown *bool
	NotifyOnWebhookFail   *bool
	APIDocsEnabled        *bool
}

type SettingsService struct {
	repo  *repository.SettingsRepository
	caddy *caddysvc.Client
}

func NewSettingsService(db *gorm.DB, caddy *caddysvc.Client) *SettingsService {
	return &SettingsService{
		repo:  repository.NewSettingsRepository(db),
		caddy: caddy,
	}
}

func (s *SettingsService) Get() (models.SystemSettings, error) {
	return s.repo.Get()
}

func (s *SettingsService) Update(input SettingsUpdateInput) (models.SystemSettings, error) {
	var settings models.SystemSettings
	if err := s.repo.FirstOrCreate(&settings); err != nil {
		return models.SystemSettings{}, err
	}

	helpers.ApplyIfSet(&settings.InstanceName, input.InstanceName)
	helpers.ApplyIfSet(&settings.InstanceURL, input.InstanceURL)
	helpers.ApplyIfSet(&settings.RegistrationMode, input.RegistrationMode)
	helpers.ApplyIfSet(&settings.CaddyBaseDomain, input.CaddyBaseDomain)
	helpers.ApplyIfSet(&settings.SMTPHost, input.SMTPHost)
	helpers.ApplyIfSet(&settings.SMTPPort, input.SMTPPort)
	helpers.ApplyIfSet(&settings.SMTPUsername, input.SMTPUsername)
	helpers.ApplyIfSet(&settings.SMTPPassword, input.SMTPPassword)
	helpers.ApplyIfSet(&settings.SMTPFrom, input.SMTPFrom)
	helpers.ApplyIfSet(&settings.SMTPTLSEnabled, input.SMTPTLSEnabled)
	helpers.ApplyIfSet(&settings.SessionTimeoutHours, input.SessionTimeoutHours)
	helpers.ApplyIfSet(&settings.NotificationsEnabled, input.NotificationsEnabled)
	helpers.ApplyIfSet(&settings.SlackWebhookURL, input.SlackWebhookURL)
	helpers.ApplyIfSet(&settings.DiscordWebhookURL, input.DiscordWebhookURL)
	helpers.ApplyIfSet(&settings.NotifyOnDeploy, input.NotifyOnDeploy)
	helpers.ApplyIfSet(&settings.NotifyOnContainerDown, input.NotifyOnContainerDown)
	helpers.ApplyIfSet(&settings.NotifyOnWebhookFail, input.NotifyOnWebhookFail)
	helpers.ApplyIfSet(&settings.APIDocsEnabled, input.APIDocsEnabled)

	if err := s.repo.Save(&settings); err != nil {
		return models.SystemSettings{}, fmt.Errorf("update settings: %w", err)
	}

	if input.CaddyBaseDomain != nil && s.caddy != nil {
		s.caddy.SetBaseDomain(*input.CaddyBaseDomain)
	}

	return settings, nil
}
