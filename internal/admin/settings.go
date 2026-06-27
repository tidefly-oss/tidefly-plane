package admin

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type getSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) getSettings(_ context.Context, _ *struct{}) (*getSettingsOutput, error) {
	s, err := h.settings.Get()
	if err != nil {
		return nil, err
	}
	return &getSettingsOutput{Body: s}, nil
}

type updateSettingsInput struct {
	Body struct {
		InstanceName                 *string `json:"instance_name,omitempty"`
		InstanceURL                  *string `json:"instance_url,omitempty"`
		RegistrationMode             *string `json:"registration_mode,omitempty"`
		CaddyBaseDomain              *string `json:"caddy_base_domain,omitempty"`
		SMTPHost                     *string `json:"smtp_host,omitempty"`
		SMTPPort                     *int    `json:"smtp_port,omitempty"`
		SMTPUsername                 *string `json:"smtp_username,omitempty"`
		SMTPPassword                 *string `json:"smtp_password,omitempty"`
		SMTPFrom                     *string `json:"smtp_from,omitempty"`
		SMTPTLSEnabled               *bool   `json:"smtp_tls_enabled,omitempty"`
		NotificationsEnabled         *bool   `json:"notifications_enabled,omitempty"`
		ExternalNotificationsEnabled *bool   `json:"external_notifications_enabled,omitempty"`
		SlackWebhookURL              *string `json:"slack_webhook_url,omitempty"`
		DiscordWebhookURL            *string `json:"discord_webhook_url,omitempty"`
		NotifyOnDeploy               *bool   `json:"notify_on_deploy,omitempty"`
		NotifyOnContainerDown        *bool   `json:"notify_on_container_down,omitempty"`
		NotifyOnWebhookFail          *bool   `json:"notify_on_webhook_fail,omitempty"`
		APIDocsEnabled               *bool   `json:"api_docs_enabled,omitempty"`
	}
}

type updateSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) updateSettings(ctx context.Context, input *updateSettingsInput) (*updateSettingsOutput, error) {
	s, err := h.settings.Update(SettingsUpdateInput{
		InstanceName:                 input.Body.InstanceName,
		InstanceURL:                  input.Body.InstanceURL,
		RegistrationMode:             input.Body.RegistrationMode,
		CaddyBaseDomain:              input.Body.CaddyBaseDomain,
		SMTPHost:                     input.Body.SMTPHost,
		SMTPPort:                     input.Body.SMTPPort,
		SMTPUsername:                 input.Body.SMTPUsername,
		SMTPPassword:                 input.Body.SMTPPassword,
		SMTPFrom:                     input.Body.SMTPFrom,
		SMTPTLSEnabled:               input.Body.SMTPTLSEnabled,
		NotificationsEnabled:         input.Body.NotificationsEnabled,
		ExternalNotificationsEnabled: input.Body.ExternalNotificationsEnabled,
		SlackWebhookURL:              input.Body.SlackWebhookURL,
		DiscordWebhookURL:            input.Body.DiscordWebhookURL,
		NotifyOnDeploy:               input.Body.NotifyOnDeploy,
		NotifyOnContainerDown:        input.Body.NotifyOnContainerDown,
		NotifyOnWebhookFail:          input.Body.NotifyOnWebhookFail,
		APIDocsEnabled:               input.Body.APIDocsEnabled,
	})
	h.log.Audit(ctx, logger.AuditEntry{
		Action:  logger.AuditAdminSettingsUpdate,
		Success: err == nil,
	})
	if err != nil {
		return nil, err
	}
	return &updateSettingsOutput{Body: s}, nil
}

type testNotificationInput struct {
	Channel string `path:"channel" doc:"Channel to test: slack, discord, email"`
}

func (h *Handler) testNotification(ctx context.Context, input *testNotificationInput) (*struct{}, error) {
	if err := h.notifier.Test(ctx, input.Channel); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return nil, nil
}
