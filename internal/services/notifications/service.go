package notifications

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// normalizeRe strips dynamic parts (timestamps, PIDs, addresses, line numbers)
// so semantically identical messages always produce the same fingerprint.
var normalizeRe = regexp.MustCompile(
	`\b(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[^\s]*` + // ISO timestamps
		`|(?:\d{1,3}\.){3}\d{1,3}` + // IPv4
		`|0x[0-9a-fA-F]+` + // hex addresses
		`|pid\s*\d+` + // pid 1234
		`|\b\d{5,}\b` + // long numbers (port, fd, etc.)
		`)\b`,
)

// SSEClient represents one connected browser tab listening for notifications.
type SSEClient struct {
	ch   chan []byte
	done <-chan struct{}
}

// Service handles creation, deduplication, acknowledgement and SSE broadcasting.
type Service struct {
	db      *gorm.DB
	mu      sync.RWMutex
	clients map[string]*SSEClient // key = random connection id
}

func New(db *gorm.DB) *Service {
	return &Service{
		db:      db,
		clients: make(map[string]*SSEClient),
	}
}

// ── SSE Hub ─────────────────────────────────────────────────────────────────

// Subscribe registers a new SSE client and returns its channel + unsubscribe func.
func (s *Service) Subscribe(ctx context.Context) (<-chan []byte, func()) {
	id := ulid.Make().String()
	ch := make(chan []byte, 32)
	client := &SSEClient{ch: ch, done: ctx.Done()}

	s.mu.Lock()
	s.clients[id] = client
	s.mu.Unlock()

	unsub := func() {
		s.mu.Lock()
		delete(s.clients, id)
		s.mu.Unlock()
		close(ch)
	}
	return ch, unsub
}

// broadcast sends a JSON-encoded notifications to all connected SSE clients.
func (s *Service) broadcast(n *models.Notification) {
	data, err := json.Marshal(n)
	if err != nil {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, c := range s.clients {
		select {
		case c.ch <- data:
		default:
			// Slow client — skip rather than block
		}
	}
}

// ── Core Logic ───────────────────────────────────────────────────────────────

// Upsert creates a new notifications or increments OccurrenceCount if one with
// the same fingerprint already exists. Only broadcasts on first occurrence or
// when the existing one was previously acknowledged (re-opened).
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

	// ON CONFLICT (fingerprint): increment count, clear ack, refresh updated_at.
	result := s.db.WithContext(ctx).
		Clauses(
			clause.OnConflict{
				Columns: []clause.Column{{Name: "fingerprint"}},
				DoUpdates: clause.Assignments(
					map[string]interface{}{
						"occurrence_count": gorm.Expr("notifications.occurrence_count + 1"),
						"updated_at":       now,
					},
				),
			},
		).
		Create(&n)

	if result.Error != nil {
		return fmt.Errorf("notifications.Upsert: %w", result.Error)
	}

	// Fetch the final row (may have been the upserted existing one).
	var final models.Notification
	if err := s.db.WithContext(ctx).
		Where("fingerprint = ?", fp).
		First(&final).Error; err == nil {
		s.broadcast(&final)
	}

	return nil
}

// List returns all unacknowledged notifications, newest first.
func (s *Service) List(ctx context.Context) ([]models.Notification, error) {
	var ns []models.Notification
	err := s.db.WithContext(ctx).
		Where("acknowledged_at IS NULL").
		Order("updated_at DESC").
		Find(&ns).Error
	return ns, err
}

// ListAll returns all notifications (including acknowledged), newest first.
func (s *Service) ListAll(ctx context.Context, limit int) ([]models.Notification, error) {
	var ns []models.Notification
	err := s.db.WithContext(ctx).
		Order("updated_at DESC").
		Limit(limit).
		Find(&ns).Error
	return ns, err
}

// Acknowledge marks a notifications as done. If id == "all", all are acknowledged.
func (s *Service) Acknowledge(ctx context.Context, id string) error {
	now := time.Now()
	q := s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("acknowledged_at IS NULL")

	if id != "all" {
		q = q.Where("id = ?", id)
	}

	return q.Update("acknowledged_at", now).Error
}

// Delete removes a notifications by id (hard delete, used by "Clear").
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&models.Notification{}).Error
}

// DeleteAll hard-deletes all acknowledged notifications (housekeeping).
func (s *Service) DeleteAll(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Where("acknowledged_at IS NOT NULL").
		Delete(&models.Notification{}).Error
}

// UnreadCount returns the number of unacknowledged notifications.
func (s *Service) UnreadCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.Notification{}).
		Where("acknowledged_at IS NULL").
		Count(&count).Error
	return count, err
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
		Clauses(
			clause.OnConflict{
				Columns: []clause.Column{{Name: "fingerprint"}},
				DoUpdates: clause.Assignments(
					map[string]interface{}{
						"occurrence_count": gorm.Expr("notifications.occurrence_count + 1"),
						"acknowledged_at":  nil,
						"updated_at":       now,
					},
				),
			},
		).
		Create(&n)
	if result.Error != nil {
		return fmt.Errorf("notifications.Publish: %w", result.Error)
	}
	var final models.Notification
	if err := s.db.WithContext(ctx).Where("fingerprint = ?", fp).First(&final).Error; err == nil {
		s.broadcast(&final)
	}
	return nil
}

func (s *Service) IsNew(ctx context.Context, fp string) bool {
	var count int64
	s.db.WithContext(ctx).Model(&models.Notification{}).
		Where("fingerprint = ? AND acknowledged_at IS NULL AND occurrence_count <= 1", fp).
		Count(&count)
	return count > 0
}

// ── Helpers ──────────────────────────────────────────────────────────────────

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
