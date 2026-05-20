package notification

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var normalizeRe = regexp.MustCompile(
	`\b(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[^\s]*` +
		`|(?:\d{1,3}\.){3}\d{1,3}` +
		`|0x[0-9a-fA-F]+` +
		`|pid\s*\d+` +
		`|\b\d{5,}\b` +
		`)\b`,
)

type Service struct {
	db  *gorm.DB
	bus *eventbus.Bus
}

func New(db *gorm.DB, bus *eventbus.Bus) *Service {
	return &Service{db: db, bus: bus}
}

func (s *Service) publish(n *models.Notification) {
	s.bus.Publish(eventbus.Event{
		Type:  eventbus.EventNotificationCreated,
		Topic: eventbus.TopicNotifications,
		Payload: eventbus.NotificationCreatedPayload{
			ID:      n.ID,
			Title:   n.ContainerName,
			Message: n.Message,
			Level:   string(n.Severity),
		},
	})
}

func (s *Service) Upsert(
	ctx context.Context,
	containerID, containerName string,
	severity models.NotificationSeverity,
	message string,
) error {
	fp := Fingerprint(containerID, string(severity), message)
	now := time.Now()
	n := models.Notification{
		ID:              ulid.Make().String(),
		ContainerID:     containerID,
		ContainerName:   containerName,
		Severity:        severity,
		Message:         truncate(message, 1024),
		Fingerprint:     fp,
		OccurrenceCount: 1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	result := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "fingerprint"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"occurrence_count": gorm.Expr("notifications.occurrence_count + 1"),
				"updated_at":       now,
			}),
		}).
		Create(&n)
	if result.Error != nil {
		return fmt.Errorf("notifications.Upsert: %w", result.Error)
	}
	var final models.Notification
	if err := s.db.WithContext(ctx).Where("fingerprint = ?", fp).First(&final).Error; err == nil {
		s.publish(&final)
	}
	return nil
}

func (s *Service) Publish(ctx context.Context, severity models.NotificationSeverity, title, message string) error {
	fp := Fingerprint("system", string(severity), title+"|"+message)
	now := time.Now()
	n := models.Notification{
		ID:              ulid.Make().String(),
		ContainerID:     "",
		ContainerName:   "system",
		Severity:        severity,
		Message:         truncate(title+": "+message, 1024),
		Fingerprint:     fp,
		OccurrenceCount: 1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	result := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "fingerprint"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"occurrence_count": gorm.Expr("notifications.occurrence_count + 1"),
				"acknowledged_at":  nil,
				"updated_at":       now,
			}),
		}).
		Create(&n)
	if result.Error != nil {
		return fmt.Errorf("notifications.Publish: %w", result.Error)
	}
	var final models.Notification
	if err := s.db.WithContext(ctx).Where("fingerprint = ?", fp).First(&final).Error; err == nil {
		s.publish(&final)
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]models.Notification, error) {
	var ns []models.Notification
	err := s.db.WithContext(ctx).
		Where("acknowledged_at IS NULL").
		Order("updated_at DESC").
		Find(&ns).Error
	return ns, err
}

func (s *Service) ListAll(ctx context.Context, limit int) ([]models.Notification, error) {
	var ns []models.Notification
	err := s.db.WithContext(ctx).
		Order("updated_at DESC").
		Limit(limit).
		Find(&ns).Error
	return ns, err
}

func (s *Service) Acknowledge(ctx context.Context, id string) error {
	now := time.Now()
	q := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("acknowledged_at IS NULL")
	if id != "all" {
		q = q.Where("id = ?", id)
	}
	return q.Update("acknowledged_at", now).Error
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Where("id = ?", id).Delete(&models.Notification{}).Error
}

func (s *Service) DeleteAll(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Where("acknowledged_at IS NOT NULL").
		Delete(&models.Notification{}).Error
}

func (s *Service) UnreadCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.Notification{}).
		Where("acknowledged_at IS NULL").
		Count(&count).Error
	return count, err
}

func (s *Service) IsNew(ctx context.Context, fp string) bool {
	var count int64
	s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("fingerprint = ? AND acknowledged_at IS NULL AND occurrence_count <= 1", fp).
		Count(&count)
	return count > 0
}

func Fingerprint(containerID, severity, message string) string {
	normalized := normalizeRe.ReplaceAllString(message, "<X>")
	normalized = strings.TrimSpace(normalized)
	h := sha256.Sum256([]byte(containerID + "|" + severity + "|" + normalized))
	return fmt.Sprintf("%x", h)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
