package models

import (
	"time"
)

type NotificationSeverity string

const (
	SeverityInfo  NotificationSeverity = "INFO"
	SeverityFatal NotificationSeverity = "FATAL"
	SeverityError NotificationSeverity = "ERROR"
	SeverityWarn  NotificationSeverity = "WARN"
)

type Notification struct {
	ID              string               `gorm:"primaryKey;type:varchar(26)"       json:"id"`
	ContainerID     string               `gorm:"index;not null"                    json:"container_id"`
	ContainerName   string               `gorm:"not null"                          json:"container_name"`
	Severity        NotificationSeverity `gorm:"type:varchar(10);not null;index"   json:"severity"`
	Message         string               `gorm:"type:text;not null"                json:"message"`
	Fingerprint     string               `gorm:"uniqueIndex;not null"              json:"-"`
	OccurrenceCount int                  `gorm:"default:1"                         json:"occurrence_count"`
	AcknowledgedAt  *time.Time           `gorm:"index"                             json:"acknowledged_at"`
	CreatedAt       time.Time            `                                         json:"created_at"`
	UpdatedAt       time.Time            `                                         json:"updated_at"`
}
