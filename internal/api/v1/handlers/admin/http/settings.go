package http

import (
	"context"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/admin/service"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type GetSettingsInput struct{}
type GetSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) GetSettings(_ context.Context, _ *GetSettingsInput) (*GetSettingsOutput, error) {
	s, err := h.settings.Get()
	if err != nil {
		return nil, err
	}
	return &GetSettingsOutput{Body: s}, nil
}

type UpdateSettingsBody struct {
	InstanceName          *string `json:"instance_name,omitempty"`
	InstanceURL           *string `json:"instance_url,omitempty"`
	RegistrationMode      *string `json:"registration_mode,omitempty"`
	CaddyBaseDomain       *string `json:"caddy_base_domain,omitempty"`
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

type UpdateSettingsInput struct {
	Body UpdateSettingsBody
}

type UpdateSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) UpdateSettings(ctx context.Context, input *UpdateSettingsInput) (*UpdateSettingsOutput, error) {
	s, err := h.settings.Update(
		service.SettingsUpdateInput{
			InstanceName:          input.Body.InstanceName,
			InstanceURL:           input.Body.InstanceURL,
			RegistrationMode:      input.Body.RegistrationMode,
			CaddyBaseDomain:       input.Body.CaddyBaseDomain,
			SMTPHost:              input.Body.SMTPHost,
			SMTPPort:              input.Body.SMTPPort,
			SMTPUsername:          input.Body.SMTPUsername,
			SMTPPassword:          input.Body.SMTPPassword,
			SMTPFrom:              input.Body.SMTPFrom,
			SMTPTLSEnabled:        input.Body.SMTPTLSEnabled,
			SessionTimeoutHours:   input.Body.SessionTimeoutHours,
			NotificationsEnabled:  input.Body.NotificationsEnabled,
			SlackWebhookURL:       input.Body.SlackWebhookURL,
			DiscordWebhookURL:     input.Body.DiscordWebhookURL,
			NotifyOnDeploy:        input.Body.NotifyOnDeploy,
			NotifyOnContainerDown: input.Body.NotifyOnContainerDown,
			NotifyOnWebhookFail:   input.Body.NotifyOnWebhookFail,
		},
	)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:  logger.AuditAdminSettingsUpdate,
			Success: err == nil,
		},
	)
	if err != nil {
		return nil, err
	}
	return &UpdateSettingsOutput{Body: s}, nil
}
