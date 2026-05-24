package http

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/webhooks/helpers"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/webhook"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type webhookResponse struct {
	models.Webhook
	SecretPlain string `json:"secret,omitempty"`
	URL         string `json:"url"`
}

type WebhookListInput struct {
	PID string `path:"pid"`
}
type WebhookListOutput struct {
	Body []webhookResponse
}

type WebhookCreateInput struct {
	PID  string `path:"pid"`
	Body struct {
		Name             string                    `json:"name" minLength:"1"`
		TriggerType      models.WebhookTriggerType `json:"trigger_type"`
		Branch           string                    `json:"branch,omitempty"`
		Provider         string                    `json:"provider,omitempty"`
		ServiceID        *string                   `json:"service_id,omitempty"`
		GitIntegrationID *string                   `json:"git_integration_id,omitempty"`
		RepoURL          string                    `json:"repo_url,omitempty"`
		TemplateSlug     string                    `json:"template_slug,omitempty"`
		FieldOverrides   string                    `json:"field_overrides,omitempty"`
	}
}
type WebhookCreateOutput struct {
	Body webhookResponse
}

type WebhookGetInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}
type WebhookGetOutput struct {
	Body webhookResponse
}

type WebhookUpdateInput struct {
	PID  string `path:"pid"`
	ID   string `path:"id"`
	Body struct {
		Name           *string `json:"name,omitempty"`
		Branch         *string `json:"branch,omitempty"`
		Active         *bool   `json:"active,omitempty"`
		FieldOverrides *string `json:"field_overrides,omitempty"`
	}
}
type WebhookUpdateOutput struct {
	Body webhookResponse
}

type WebhookRotateSecretInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}
type WebhookRotateSecretOutput struct {
	Body struct {
		Secret string `json:"secret"`
	}
}

type WebhookDeleteInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}

type WebhookDeliveriesInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}
type WebhookDeliveriesOutput struct {
	Body []models.WebhookDelivery
}

func (h *Handler) List(ctx context.Context, input *WebhookListInput) (*WebhookListOutput, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	webhooks, err := h.webhooks.List(ctx, input.PID)
	if err != nil {
		return nil, err
	}
	resp := make([]webhookResponse, len(webhooks))
	for i, wh := range webhooks {
		resp[i] = webhookResponse{Webhook: wh, URL: helpers.BuildURL(ctx, wh.ID)}
	}
	return &WebhookListOutput{Body: resp}, nil
}

func (h *Handler) Create(ctx context.Context, input *WebhookCreateInput) (*WebhookCreateOutput, error) {
	user, err := h.webhooks.CheckProjectAccess(ctx, input.PID)
	if err != nil {
		return nil, err
	}
	rawSecret, err := helpers.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	encSecret, err := h.svc.EncryptSecret(rawSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}
	provider := input.Body.Provider
	if provider == "" {
		provider = string(webhook.ProviderGeneric)
	}
	wh := models.Webhook{
		ID: uuid.New().String(), ProjectID: input.PID,
		CreatedBy: user.ID, Name: input.Body.Name, Active: true,
		Secret: encSecret, Branch: input.Body.Branch, Provider: provider,
		TriggerType: input.Body.TriggerType, ServiceID: input.Body.ServiceID,
		GitIntegrationID: input.Body.GitIntegrationID, RepoURL: input.Body.RepoURL,
		TemplateSlug: input.Body.TemplateSlug, FieldOverrides: input.Body.FieldOverrides,
	}
	if err := h.webhooks.Create(ctx, &wh); err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditWebhookCreate,
		ResourceID: wh.ID,
		Success:    true,
		Details:    "name=" + wh.Name + " project=" + input.PID,
	})
	return &WebhookCreateOutput{Body: webhookResponse{
		Webhook:     wh,
		SecretPlain: rawSecret,
		URL:         helpers.BuildURL(ctx, wh.ID),
	}}, nil
}

func (h *Handler) Get(ctx context.Context, input *WebhookGetInput) (*WebhookGetOutput, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.webhooks.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	return &WebhookGetOutput{Body: webhookResponse{Webhook: *wh, URL: helpers.BuildURL(ctx, wh.ID)}}, nil
}

func (h *Handler) Update(ctx context.Context, input *WebhookUpdateInput) (*WebhookUpdateOutput, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.webhooks.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if input.Body.Name != nil {
		updates["name"] = *input.Body.Name
	}
	if input.Body.Branch != nil {
		updates["branch"] = *input.Body.Branch
	}
	if input.Body.Active != nil {
		updates["active"] = *input.Body.Active
	}
	if input.Body.FieldOverrides != nil {
		updates["field_overrides"] = *input.Body.FieldOverrides
	}
	if err := h.webhooks.Update(ctx, wh, updates); err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditWebhookUpdate, ResourceID: wh.ID, Success: true})
	return &WebhookUpdateOutput{Body: webhookResponse{Webhook: *wh, URL: helpers.BuildURL(ctx, wh.ID)}}, nil
}

func (h *Handler) RotateSecret(ctx context.Context, input *WebhookRotateSecretInput) (*WebhookRotateSecretOutput, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.webhooks.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	rawSecret, err := helpers.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	encSecret, err := h.svc.EncryptSecret(rawSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}
	if err := h.webhooks.Update(ctx, wh, map[string]any{"secret": encSecret}); err != nil {
		return nil, fmt.Errorf("save secret: %w", err)
	}
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditWebhookRotate, ResourceID: wh.ID, Success: true})
	out := &WebhookRotateSecretOutput{}
	out.Body.Secret = rawSecret
	return out, nil
}

func (h *Handler) Delete(ctx context.Context, input *WebhookDeleteInput) (*struct{}, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.webhooks.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	if err := h.webhooks.Delete(ctx, wh); err != nil {
		return nil, fmt.Errorf("delete webhook: %w", err)
	}
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditWebhookDelete,
		ResourceID: wh.ID,
		Success:    true,
		Details:    "name=" + wh.Name,
	})
	return nil, nil
}

func (h *Handler) Deliveries(ctx context.Context, input *WebhookDeliveriesInput) (*WebhookDeliveriesOutput, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	deliveries, err := h.webhooks.Deliveries(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	return &WebhookDeliveriesOutput{Body: deliveries}, nil
}
