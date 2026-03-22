package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

func (h *Handler) HandleLogsRetention(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		LogRetentionDays          int `json:"log_retention_days"`
		AuditRetentionDays        int `json:"audit_retention_days"`
		NotificationRetentionDays int `json:"notification_retention_days"`
	}
	_ = json.Unmarshal(t.Payload(), &payload)
	if payload.LogRetentionDays <= 0 {
		payload.LogRetentionDays = 30
	}
	if payload.AuditRetentionDays <= 0 {
		payload.AuditRetentionDays = 90
	}
	if payload.NotificationRetentionDays <= 0 {
		payload.NotificationRetentionDays = 30
	}

	appLogCutoff := time.Now().AddDate(0, 0, -payload.LogRetentionDays)
	result := h.db.WithContext(ctx).
		Where("created_at < ?", appLogCutoff).
		Delete(&models.AppLog{})
	if result.Error != nil {
		return fmt.Errorf("log retention: delete app logs: %w", result.Error)
	}

	auditCutoff := time.Now().AddDate(0, 0, -payload.AuditRetentionDays)
	auditResult := h.db.WithContext(ctx).
		Where("created_at < ?", auditCutoff).
		Delete(&models.AuditLog{})
	if auditResult.Error != nil {
		return fmt.Errorf("log retention: delete audit logs: %w", auditResult.Error)
	}

	notifCutoff := time.Now().AddDate(0, 0, -payload.NotificationRetentionDays)
	notifResult := h.db.WithContext(ctx).
		Where("acknowledged_at IS NOT NULL AND acknowledged_at < ?", notifCutoff).
		Delete(&models.Notification{})
	if notifResult.Error != nil {
		return fmt.Errorf("log retention: delete notifications: %w", notifResult.Error)
	}

	h.log.Info(
		"jobs", fmt.Sprintf(
			"retention: deleted %d app logs, %d audit logs, %d notifications",
			result.RowsAffected, auditResult.RowsAffected, notifResult.RowsAffected,
		),
	)
	h.metrics.IncJob(TaskLogsRetention)
	return nil
}
