package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-backend/internal/jobs"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
)

type Handler struct {
	db    *gorm.DB
	queue *asynq.Client
	log   *logger.Logger
	svc   *webhook.Service
}

func New(db *gorm.DB, queue *asynq.Client, log *logger.Logger, svc *webhook.Service) *Handler {
	return &Handler{db: db, queue: queue, log: log, svc: svc}
}

type webhookResponse struct {
	models.Webhook
	SecretPlain string `json:"secret,omitempty"`
	URL         string `json:"url"`
}

// ── Access check ──────────────────────────────────────────────────────────────

func (h *Handler) checkProjectAccess(ctx context.Context, projectID string) (*models.User, error) {
	u := middleware.UserFromHumaCtx(ctx)
	if u == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	user, ok := u.(*models.User)
	if !ok || user == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if user.IsAdmin() {
		return user, nil
	}
	var count int64
	if err := h.db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, user.ID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}
	if count == 0 {
		return nil, huma.Error403Forbidden("not a member of this project")
	}
	return user, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

type ListInput struct {
	PID string `path:"pid" doc:"Project ID"`
}
type ListOutput struct {
	Body []webhookResponse
}

func (h *Handler) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	if _, err := h.checkProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	var webhooks []models.Webhook
	if err := h.db.WithContext(ctx).
		Where("project_id = ?", input.PID).
		Order("created_at DESC").Find(&webhooks).Error; err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	resp := make([]webhookResponse, len(webhooks))
	for i, wh := range webhooks {
		resp[i] = webhookResponse{Webhook: wh, URL: buildURL(ctx, wh.ID)}
	}
	return &ListOutput{Body: resp}, nil
}

// ── Create ────────────────────────────────────────────────────────────────────

type CreateInput struct {
	PID  string `path:"pid" doc:"Project ID"`
	Body struct {
		Name             string                    `json:"name" minLength:"1" doc:"Webhook name"`
		TriggerType      models.WebhookTriggerType `json:"trigger_type" doc:"Trigger type"`
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

func (h *Handler) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
	user, err := h.checkProjectAccess(ctx, input.PID)
	if err != nil {
		return nil, err
	}

	rawSecret, err := generateSecret()
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
	if err := h.db.WithContext(ctx).Create(&wh).Error; err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditWebhookCreate, ResourceID: wh.ID, Success: true,
			Details: "name=" + wh.Name + " project=" + input.PID,
		},
	)
	return &CreateOutput{Body: webhookResponse{Webhook: wh, SecretPlain: rawSecret, URL: buildURL(ctx, wh.ID)}}, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type GetInput struct {
	PID string `path:"pid" doc:"Project ID"`
	ID  string `path:"id" doc:"Webhook ID"`
}
type GetOutput struct {
	Body webhookResponse
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if _, err := h.checkProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.loadCtx(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	return &GetOutput{Body: webhookResponse{Webhook: *wh, URL: buildURL(ctx, wh.ID)}}, nil
}

// ── Update ────────────────────────────────────────────────────────────────────

type UpdateInput struct {
	PID  string `path:"pid" doc:"Project ID"`
	ID   string `path:"id" doc:"Webhook ID"`
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

func (h *Handler) Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error) {
	if _, err := h.checkProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.loadCtx(ctx, input.ID, input.PID)
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
	if err := h.db.WithContext(ctx).Model(wh).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditWebhookUpdate, ResourceID: wh.ID, Success: true})
	return &UpdateOutput{Body: webhookResponse{Webhook: *wh, URL: buildURL(ctx, wh.ID)}}, nil
}

// ── RotateSecret ──────────────────────────────────────────────────────────────

type RotateSecretInput struct {
	PID string `path:"pid" doc:"Project ID"`
	ID  string `path:"id" doc:"Webhook ID"`
}
type RotateSecretOutput struct {
	Body struct {
		Secret string `json:"secret"`
	}
}

func (h *Handler) RotateSecret(ctx context.Context, input *RotateSecretInput) (*RotateSecretOutput, error) {
	if _, err := h.checkProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.loadCtx(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	rawSecret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	encSecret, err := h.svc.EncryptSecret(rawSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}
	if err := h.db.WithContext(ctx).Model(wh).Update("secret", encSecret).Error; err != nil {
		return nil, fmt.Errorf("save secret: %w", err)
	}
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditWebhookRotate, ResourceID: wh.ID, Success: true})
	out := &RotateSecretOutput{}
	out.Body.Secret = rawSecret
	return out, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type DeleteInput struct {
	PID string `path:"pid" doc:"Project ID"`
	ID  string `path:"id" doc:"Webhook ID"`
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	if _, err := h.checkProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	wh, err := h.loadCtx(ctx, input.ID, input.PID)
	if err != nil {
		return nil, err
	}
	if err := h.db.WithContext(ctx).Delete(wh).Error; err != nil {
		return nil, fmt.Errorf("delete webhook: %w", err)
	}
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditWebhookDelete, ResourceID: wh.ID, Success: true,
			Details: "name=" + wh.Name,
		},
	)
	return nil, nil
}

// ── Deliveries ────────────────────────────────────────────────────────────────

type DeliveriesInput struct {
	PID string `path:"pid" doc:"Project ID"`
	ID  string `path:"id" doc:"Webhook ID"`
}
type DeliveriesOutput struct {
	Body []models.WebhookDelivery
}

func (h *Handler) Deliveries(ctx context.Context, input *DeliveriesInput) (*DeliveriesOutput, error) {
	if _, err := h.checkProjectAccess(ctx, input.PID); err != nil {
		return nil, err
	}
	var deliveries []models.WebhookDelivery
	if err := h.db.WithContext(ctx).
		Where("webhook_id = ?", input.ID).
		Order("created_at DESC").Limit(50).
		Find(&deliveries).Error; err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	return &DeliveriesOutput{Body: deliveries}, nil
}

// ── Receive (bleibt Echo — braucht rohen Request-Body für HMAC) ───────────────

func (h *Handler) Receive(c *echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	var wh models.Webhook
	if err := h.db.WithContext(ctx).First(&wh, "id = ? AND active = true", id).Error; err != nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	rawSecret, err := h.svc.DecryptSecret(wh.Secret)
	if err != nil {
		h.log.Error("webhooks", "secret decrypt failed", err)
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	provider := webhook.Provider(wh.Provider)
	payload, err := webhook.VerifyAndParse(c.Request(), provider, rawSecret)
	if err != nil {
		h.log.Warn(
			"webhooks", "signature invalid",
			fmt.Sprintf("webhook_id=%s err=%s", id, err.Error()),
		)
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
	}

	if payload.IsPing() {
		return c.JSON(http.StatusOK, map[string]string{"status": "pong"})
	}
	if !webhook.MatchesBranch(wh.Branch, payload.Branch) {
		return c.JSON(http.StatusOK, map[string]string{"status": "skipped", "reason": "branch filter"})
	}

	delivery := models.WebhookDelivery{
		ID: uuid.New().String(), WebhookID: wh.ID,
		Provider: string(provider), EventType: payload.EventType,
		Branch: payload.Branch, Commit: payload.Commit,
		CommitMsg: payload.CommitMsg, PushedBy: payload.PushedBy,
		RepoURL: payload.RepoURL, Status: models.WebhookStatusPending,
	}
	h.db.WithContext(ctx).Create(&delivery)

	if err := jobs.EnqueueWebhookDeploy(h.queue, wh.ID, delivery.ID, *payload); err != nil {
		h.db.WithContext(ctx).Model(&delivery).Updates(
			map[string]any{
				"status": models.WebhookStatusFailed, "error_msg": err.Error(),
			},
		)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "enqueue failed"})
	}

	h.db.WithContext(ctx).Model(&wh).Updates(
		map[string]any{
			"last_triggered_at": time.Now(),
			"last_status":       models.WebhookStatusPending,
		},
	)
	return c.JSON(http.StatusAccepted, map[string]string{"status": "accepted", "delivery_id": delivery.ID})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) loadCtx(ctx context.Context, id, projectID string) (*models.Webhook, error) {
	var wh models.Webhook
	if err := h.db.WithContext(ctx).
		First(&wh, "id = ? AND project_id = ?", id, projectID).Error; err != nil {
		return nil, huma.Error404NotFound("webhook not found")
	}
	return &wh, nil
}

func buildURL(ctx context.Context, id string) string {
	if host, ok := ctx.Value("request_host").(string); ok && host != "" {
		return "https://" + host + "/webhooks/" + id
	}
	return "/webhooks/" + id
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
