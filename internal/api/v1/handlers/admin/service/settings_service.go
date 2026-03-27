package service

import (
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/helpers"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/services/caddy"
	"gorm.io/gorm"
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
}

type SettingsService struct {
	db    *gorm.DB
	caddy *caddysvc.Client
}

func NewSettingsService(db *gorm.DB, caddy *caddysvc.Client) *SettingsService {
	return &SettingsService{db: db, caddy: caddy}
}

func (s *SettingsService) Get() (models.SystemSettings, error) {
	var settings models.SystemSettings
	if err := s.db.First(&settings).Error; err != nil {
		return models.SystemSettings{}, nil
	}
	return settings, nil
}

func (s *SettingsService) Update(input SettingsUpdateInput) (models.SystemSettings, error) {
	var settings models.SystemSettings
	s.db.FirstOrCreate(&settings)

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

	if err := s.db.Save(&settings).Error; err != nil {
		return models.SystemSettings{}, fmt.Errorf("update settings: %w", err)
	}

	// Update Caddy base domain live if changed
	if input.CaddyBaseDomain != nil && s.caddy != nil {
		s.caddy.SetBaseDomain(*input.CaddyBaseDomain)
	}

	return settings, nil
}
