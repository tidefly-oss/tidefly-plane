package webhook

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
)

type webhookResponse struct {
	models.Webhook
	SecretPlain string `json:"secret,omitempty"`
	URL         string `json:"url"`
}

type listInput struct {
	PID string `path:"pid"`
}

type listOutput struct {
	Body []webhookResponse
}

type webhookCreateInput struct {
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

type createOutput struct {
	Body webhookResponse
}

type getInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}

type getOutput struct {
	Body webhookResponse
}

type webhookUpdateInput struct {
	PID  string `path:"pid"`
	ID   string `path:"id"`
	Body struct {
		Name           *string `json:"name,omitempty"`
		Branch         *string `json:"branch,omitempty"`
		Active         *bool   `json:"active,omitempty"`
		FieldOverrides *string `json:"field_overrides,omitempty"`
	}
}

type updateOutput struct {
	Body webhookResponse
}

type rotateSecretInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}

type rotateSecretOutput struct {
	Body struct {
		Secret string `json:"secret"`
	}
}

type deleteInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}

type deliveriesInput struct {
	PID string `path:"pid"`
	ID  string `path:"id"`
}

type deliveriesOutput struct {
	Body []models.WebhookDelivery
}

func (h *Handler) list(ctx context.Context, input *listInput) (*listOutput, error) {
	if _, err := h.store.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	webhooks, err := h.store.List(ctx, input.PID)
	if err != nil {
		return nil, err
	}
	resp := make([]webhookResponse, len(webhooks))
	for i, wh := range webhooks {
		resp[i] = webhookResponse{Webhook: wh, URL: BuildURL(ctx, wh.ID)}
	}
	return &listOutput{Body: resp}, nil
}

func (h *Handler) create(ctx context.Context, input *webhookCreateInput) (*createOutput, error) {
	user, err := h.store.CheckProjectAccess(ctx, input.PID)
	if err != nil {
		return nil, err
	}
	rawSecret, err := GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	encSecret, err := h.svc.EncryptSecret(rawSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}
	provider := input.Body.Provider
	if provider == "" {
		provider = string(ProviderGeneric)
	}
	wh := models.Webhook{
		ID: uuid.New().String(), ProjectID: input.PID,
		CreatedBy: user.ID, Name: input.Body.Name, Active: true,
		Secret: encSecret, Branch: input.Body.Branch, Provider: provider,
		TriggerType: input.Body.TriggerType, ServiceID: input.Body.ServiceID,
		GitIntegrationID: input.Body.GitIntegrationID, RepoURL: input.Body.RepoURL,
		TemplateSlug: input.Body.TemplateSlug, FieldOverrides: input.Body.FieldOverrides,
	}
	if err := h.store.Create(ctx, &wh); err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditWebhookCreate,
		ResourceID: wh.ID,
		Success:    true,
		Details:    "name=" + wh.Name + " project=" + input.PID,
	})
	return &createOutput{Body: webhookResponse{
		Webhook:     wh,
		SecretPlain: rawSecret,
		URL:         BuildURL(ctx, wh.ID),
	}}, nil
}

func (h *Handler) get(ctx context.Context, input *getInput) (*getOutput, error) {
	if _, err := h.store.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.store.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	return &getOutput{Body: webhookResponse{Webhook: *wh, URL: BuildURL(ctx, wh.ID)}}, nil
}

func (h *Handler) update(ctx context.Context, input *webhookUpdateInput) (*updateOutput, error) {
	if _, err := h.store.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.store.Load(ctx, input.ID, input.PID)
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
	if err := h.store.Update(ctx, wh, updates); err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}
	h.log.Audit(ctx, _logger.AuditEntry{Action: _logger.AuditWebhookUpdate, ResourceID: wh.ID, Success: true})
	return &updateOutput{Body: webhookResponse{Webhook: *wh, URL: BuildURL(ctx, wh.ID)}}, nil
}

func (h *Handler) rotateSecret(ctx context.Context, input *rotateSecretInput) (*rotateSecretOutput, error) {
	if _, err := h.store.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.store.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	rawSecret, err := GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	encSecret, err := h.svc.EncryptSecret(rawSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}
	if err := h.store.Update(ctx, wh, map[string]any{"secret": encSecret}); err != nil {
		return nil, fmt.Errorf("save secret: %w", err)
	}
	h.log.Audit(ctx, _logger.AuditEntry{Action: _logger.AuditWebhookRotate, ResourceID: wh.ID, Success: true})
	out := &rotateSecretOutput{}
	out.Body.Secret = rawSecret
	return out, nil
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	if _, err := h.store.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.store.Load(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	if err := h.store.Delete(ctx, wh); err != nil {
		return nil, fmt.Errorf("delete webhook: %w", err)
	}
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditWebhookDelete,
		ResourceID: wh.ID,
		Success:    true,
		Details:    "name=" + wh.Name,
	})
	return nil, nil
}

func (h *Handler) deliveries(ctx context.Context, input *deliveriesInput) (*deliveriesOutput, error) {
	if _, err := h.store.CheckProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	deliveries, err := h.store.Deliveries(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	return &deliveriesOutput{Body: deliveries}, nil
}
