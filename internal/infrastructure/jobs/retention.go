package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

func (h *Handler) HandleLogsRetention(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		AuditRetentionDays        int `json:"audit_retention_days"`
		NotificationRetentionDays int `json:"notification_retention_days"`
	}
	_ = json.Unmarshal(t.Payload(), &payload)
	if payload.AuditRetentionDays <= 0 {
		payload.AuditRetentionDays = 90
	}
	if payload.NotificationRetentionDays <= 0 {
		payload.NotificationRetentionDays = 30
	}

	// ── App Logs — level-based retention ─────────────────────────────────────
	// INFO  → 3 days  (deployment trail, self-healing recovery)
	// WARN  → 7 days  (unexpected stops, client errors)
	// ERROR → 30 days (failures requiring investigation)
	infoCutoff := time.Now().AddDate(0, 0, -3)
	warnCutoff := time.Now().AddDate(0, 0, -7)
	errorCutoff := time.Now().AddDate(0, 0, -30)

	infoResult := h.db.WithContext(ctx).
		Where("level = 'INFO' AND created_at < ?", infoCutoff).
		Delete(&models.AppLog{})
	if infoResult.Error != nil {
		return fmt.Errorf("log retention: delete INFO logs: %w", infoResult.Error)
	}

	warnResult := h.db.WithContext(ctx).
		Where("level = 'WARN' AND created_at < ?", warnCutoff).
		Delete(&models.AppLog{})
	if warnResult.Error != nil {
		return fmt.Errorf("log retention: delete WARN logs: %w", warnResult.Error)
	}

	errorResult := h.db.WithContext(ctx).
		Where("level = 'ERROR' AND created_at < ?", errorCutoff).
		Delete(&models.AppLog{})
	if errorResult.Error != nil {
		return fmt.Errorf("log retention: delete ERROR logs: %w", errorResult.Error)
	}

	// ── Audit Logs ────────────────────────────────────────────────────────────
	auditCutoff := time.Now().AddDate(0, 0, -payload.AuditRetentionDays)
	auditResult := h.db.WithContext(ctx).
		Where("created_at < ?", auditCutoff).
		Delete(&models.AuditLog{})
	if auditResult.Error != nil {
		return fmt.Errorf("log retention: delete audit logs: %w", auditResult.Error)
	}

	// ── Notifications — acknowledged only ─────────────────────────────────────
	notifCutoff := time.Now().AddDate(0, 0, -payload.NotificationRetentionDays)
	notifResult := h.db.WithContext(ctx).
		Where("acknowledged_at IS NOT NULL AND acknowledged_at < ?", notifCutoff).
		Delete(&models.Notification{})
	if notifResult.Error != nil {
		return fmt.Errorf("log retention: delete notifications: %w", notifResult.Error)
	}

	h.log.Info(
		"jobs", fmt.Sprintf(
			"retention: deleted %d INFO / %d WARN / %d ERROR app logs, %d audit logs, %d notifications",
			infoResult.RowsAffected, warnResult.RowsAffected, errorResult.RowsAffected,
			auditResult.RowsAffected, notifResult.RowsAffected,
		),
	)
	h.metrics.IncJob(TaskLogsRetention)
	return nil
}
