package http

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/webhooks/helpers"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
)

type webhookResponse struct {
	models.Webhook
	SecretPlain string `json:"secret,omitempty"`
	URL         string `json:"url"`
}

type ListInput struct {
	PID string `path:"pid"`
}
type ListOutput struct {
	Body []webhookResponse
}

type CreateInput struct {
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
type CreateOutput struct {
	Body webhookResponse
}

type GetInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}
type GetOutput struct {
	Body webhookResponse
}

type UpdateInput struct {
	PID  string `path:"pid"`
	ID   string `path:"id"`
	Body struct {
		Name           *string `json:"name,omitempty"`
		Branch         *string `json:"branch,omitempty"`
		Active         *bool   `json:"active,omitempty"`
		FieldOverrides *string `json:"field_overrides,omitempty"`
	}
}
type UpdateOutput struct {
	Body webhookResponse
}

type RotateSecretInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}
type RotateSecretOutput struct {
	Body struct {
		Secret string `json:"secret"`
	}
}

type DeleteInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}

type DeliveriesInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}
type DeliveriesOutput struct {
	Body []models.WebhookDelivery
}

func (h *Handler) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
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
	return &ListOutput{Body: resp}, nil
}

func (h *Handler) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
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
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditWebhookCreate,
			ResourceID: wh.ID,
			Success:    true,
			Details:    "name=" + wh.Name + " project=" + input.PID,
		},
	)
	return &CreateOutput{
		Body: webhookResponse{
			Webhook:     wh,
			SecretPlain: rawSecret,
			URL:         helpers.BuildURL(ctx, wh.ID),
		},
	}, nil
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.webhooks.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	return &GetOutput{Body: webhookResponse{Webhook: *wh, URL: helpers.BuildURL(ctx, wh.ID)}}, nil
}

func (h *Handler) Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error) {
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
	return &UpdateOutput{Body: webhookResponse{Webhook: *wh, URL: helpers.BuildURL(ctx, wh.ID)}}, nil
}

func (h *Handler) RotateSecret(ctx context.Context, input *RotateSecretInput) (*RotateSecretOutput, error) {
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
	out := &RotateSecretOutput{}
	out.Body.Secret = rawSecret
	return out, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
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
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditWebhookDelete,
			ResourceID: wh.ID,
			Success:    true,
			Details:    "name=" + wh.Name,
		},
	)
	return nil, nil
}

func (h *Handler) Deliveries(ctx context.Context, input *DeliveriesInput) (*DeliveriesOutput, error) {
	if _, err := h.webhooks.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	deliveries, err := h.webhooks.Deliveries(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	return &DeliveriesOutput{Body: deliveries}, nil
}
