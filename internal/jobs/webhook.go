package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
)

const TaskWebhookDeploy = "webhooks:deploy"

type WebhookDeployPayload struct {
	WebhookID  string          `json:"webhook_id"`
	DeliveryID string          `json:"delivery_id"`
	Payload    webhook.Payload `json:"payload"`
}

func EnqueueWebhookDeploy(client *asynq.Client, webhookID, deliveryID string, p webhook.Payload) error {
	data, err := json.Marshal(
		WebhookDeployPayload{
			WebhookID:  webhookID,
			DeliveryID: deliveryID,
			Payload:    p,
		},
	)
	if err != nil {
		return err
	}
	task := asynq.NewTask(
		TaskWebhookDeploy, data,
		asynq.MaxRetry(2),
		asynq.Timeout(10*time.Minute),
		asynq.Queue("webhooks"),
	)
	_, err = client.Enqueue(task)
	return err
}

type WebhookDeployHandler struct {
	db          *gorm.DB
	deployer    *deploy.Deployer
	log         *logger.Logger
	notifSvc    *notifications.Service
	notifierSvc *notifiersvc.Service
}

func NewWebhookDeployHandler(
	db *gorm.DB, deployer *deploy.Deployer, log *logger.Logger,
	notifSvc *notifications.Service, notifierSvc *notifiersvc.Service,
) *WebhookDeployHandler {
	return &WebhookDeployHandler{db: db, deployer: deployer, log: log, notifSvc: notifSvc, notifierSvc: notifierSvc}
}

func (h *WebhookDeployHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var p WebhookDeployPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	started := time.Now()

	var wh models.Webhook
	if err := h.db.WithContext(ctx).First(&wh, "id = ?", p.WebhookID).Error; err != nil {
		return h.fail(ctx, p.DeliveryID, fmt.Errorf("webhook not found: %w", err), started)
	}

	if !wh.Active {
		return h.updateDelivery(ctx, p.DeliveryID, models.WebhookStatusFailed, "webhook is disabled", "", started)
	}

	overrides, err := expandOverrides(wh.FieldOverrides, p.Payload)
	if err != nil {
		return h.fail(ctx, p.DeliveryID, fmt.Errorf("expanding overrides: %w", err), started)
	}

	var jobID string

	switch wh.TriggerType {
	case models.WebhookTriggerRedeploy:
		jobID, err = h.redeploy(ctx, &wh, p.Payload, overrides)
	case models.WebhookTriggerDeploy:
		jobID, err = h.deployFresh(ctx, &wh, p.Payload, overrides)
	default:
		err = fmt.Errorf("unknown trigger type: %s", wh.TriggerType)
	}

	if err != nil {
		return h.fail(ctx, p.DeliveryID, err, started)
	}

	now := time.Now()
	h.db.WithContext(ctx).Model(&wh).Updates(
		map[string]any{
			"last_triggered_at": now,
			"last_status":       models.WebhookStatusSuccess,
			"last_error":        "",
			"trigger_count":     gorm.Expr("trigger_count + 1"),
		},
	)

	return h.updateDelivery(ctx, p.DeliveryID, models.WebhookStatusSuccess, "", jobID, started)
}

func (h *WebhookDeployHandler) redeploy(
	ctx context.Context, wh *models.Webhook, p webhook.Payload, overrides map[string]string,
) (string, error) {
	if wh.ServiceID == nil {
		return "", fmt.Errorf("redeploy trigger requires service_id")
	}

	var svc models.Service
	if err := h.db.WithContext(ctx).First(&svc, "id = ?", *wh.ServiceID).Error; err != nil {
		return "", fmt.Errorf("service not found: %w", err)
	}

	containerID, err := h.findServiceContainer(ctx, svc.ID.String())
	if err != nil {
		return "", err
	}

	if err := h.deployer.Redeploy(
		ctx, containerID, deploy.DeployRequest{
			ProjectID: wh.ProjectID,
			Version:   p.Commit,
			Fields:    overrides,
		},
	); err != nil {
		return "", fmt.Errorf("redeploy failed: %w", err)
	}

	h.log.Info(
		"jobs", "webhook redeploy triggered",
		// use zap fields via logger.Zap() for structured logging in jobs
	)

	return uuid.New().String(), nil
}

func (h *WebhookDeployHandler) deployFresh(
	ctx context.Context, wh *models.Webhook, p webhook.Payload, overrides map[string]string,
) (string, error) {
	if wh.GitIntegrationID == nil {
		return "", fmt.Errorf("deploy trigger requires git_integration_id")
	}
	if wh.TemplateSlug == "" {
		return "", fmt.Errorf("deploy trigger requires template_slug")
	}

	fields := make(map[string]string)
	for k, v := range overrides {
		fields[k] = v
	}
	if p.Branch != "" {
		fields["GIT_BRANCH"] = p.Branch
	}
	if p.Tag != "" {
		fields["GIT_TAG"] = p.Tag
	}
	if p.Commit != "" {
		fields["GIT_COMMIT"] = p.Commit
	}

	serviceID, err := h.deployer.DeployFromTemplate(
		ctx, deploy.DeployRequest{
			ProjectID:        wh.ProjectID,
			Version:          p.Commit,
			GitIntegrationID: *wh.GitIntegrationID,
			RepoURL:          wh.RepoURL,
			Branch:           p.Branch,
			TemplateSlug:     wh.TemplateSlug,
			Fields:           fields,
		},
	)
	if err != nil {
		return "", fmt.Errorf("deploy failed: %w", err)
	}

	h.log.Info(
		"jobs", fmt.Sprintf(
			"webhook deploy triggered: template=%s service=%s branch=%s commit=%s",
			wh.TemplateSlug, serviceID, p.Branch, p.Commit,
		),
	)

	return serviceID, nil
}

// findServiceContainer finds the running container for a service by tidefly.service label.
func (h *WebhookDeployHandler) findServiceContainer(ctx context.Context, serviceID string) (string, error) {
	containers, err := h.deployer.Runtime().ListContainers(ctx, true)
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == serviceID {
			return ct.ID, nil
		}
	}
	return "", fmt.Errorf("no container found for service %s", serviceID)
}

func (h *WebhookDeployHandler) fail(ctx context.Context, deliveryID string, err error, started time.Time) error {
	_ = h.updateDelivery(ctx, deliveryID, models.WebhookStatusFailed, err.Error(), "", started)
	_ = h.notifSvc.Publish(ctx, models.SeverityError, "Webhook deploy failed", err.Error())
	h.notifierSvc.Send(
		ctx, notifiersvc.Event{
			Title:   "Webhook deploy failed",
			Message: err.Error(),
			Level:   "error",
		},
	)
	return err
}

func (h *WebhookDeployHandler) updateDelivery(
	ctx context.Context, deliveryID string, status models.WebhookStatus, errMsg, jobID string, started time.Time,
) error {
	return h.db.WithContext(ctx).Model(&models.WebhookDelivery{}).
		Where("id = ?", deliveryID).
		Updates(
			map[string]any{
				"status":      status,
				"error_msg":   errMsg,
				"job_id":      jobID,
				"duration_ms": time.Since(started).Milliseconds(),
			},
		).Error
}

func expandOverrides(raw string, p webhook.Payload) (map[string]string, error) {
	if raw == "" {
		return map[string]string{}, nil
	}
	replacer := strings.NewReplacer(
		"{{.branch}}", p.Branch,
		"{{.commit}}", p.Commit,
		"{{.tag}}", p.Tag,
	)
	expanded := replacer.Replace(raw)

	var result map[string]string
	if err := json.Unmarshal([]byte(expanded), &result); err != nil {
		return nil, fmt.Errorf("invalid field_overrides JSON: %w", err)
	}
	return result, nil
}
