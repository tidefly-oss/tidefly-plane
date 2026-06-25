package models

import "time"

// ── Notifications ─────────────────────────────────────────────────────────────

type NotificationSeverity string

const (
	SeverityInfo  NotificationSeverity = "INFO"
	SeverityWarn  NotificationSeverity = "WARN"
	SeverityError NotificationSeverity = "ERROR"
	SeverityFatal NotificationSeverity = "FATAL"
)

// Notification is an in-app alert — deduplicated by Fingerprint.
type Notification struct {
	ID              string               `gorm:"primaryKey;type:varchar(26)"     json:"id"`
	ContainerID     string               `gorm:"index;not null"                  json:"container_id"`
	ContainerName   string               `gorm:"not null"                        json:"container_name"`
	Severity        NotificationSeverity `gorm:"type:varchar(10);not null;index" json:"severity"`
	Message         string               `gorm:"type:text;not null"              json:"message"`
	Fingerprint     string               `gorm:"uniqueIndex;not null"            json:"-"`
	OccurrenceCount int                  `gorm:"default:1"                       json:"occurrence_count"`
	AcknowledgedAt  *time.Time           `gorm:"index"                           json:"acknowledged_at"`
	CreatedAt       time.Time            `                                       json:"created_at"`
	UpdatedAt       time.Time            `                                       json:"updated_at"`
}

// ── Logs ──────────────────────────────────────────────────────────────────────

// AppLog stores structured application log entries.
// INFO is never written to DB to avoid spam — use SSE stream for INFO.
type AppLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Level       string    `gorm:"size:10;index"            json:"level"`
	Message     string    `gorm:"type:text"                json:"message"`
	Component   string    `gorm:"size:100;index"           json:"component"`
	Error       string    `gorm:"type:text"                json:"error"`
	ContainerID string    `gorm:"size:64;index"            json:"container_id"`
	CreatedAt   time.Time `gorm:"index"                    json:"created_at"`
}

// AuditLog stores security-relevant actions (DB only, never streamed).
type AuditLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Action     string    `gorm:"size:100;index"           json:"action"`
	UserID     string    `gorm:"size:36;index"            json:"user_id"`
	UserEmail  string    `gorm:"size:255"                 json:"user_email"`
	IPAddress  string    `gorm:"size:45"                  json:"ip_address"`
	UserAgent  string    `gorm:"size:500"                 json:"user_agent"`
	ResourceID string    `gorm:"size:255"                 json:"resource_id"`
	Details    string    `gorm:"type:text"                json:"details"`
	Success    bool      `                                json:"success"`
	CreatedAt  time.Time `gorm:"index"                    json:"created_at"`
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

type WebhookTriggerType string

const (
	WebhookTriggerRedeploy     WebhookTriggerType = "redeploy"
	WebhookTriggerDeploy       WebhookTriggerType = "deploy"
	WebhookTriggerUpdateNotify WebhookTriggerType = "update_notify"
)

type WebhookStatus string

const (
	WebhookStatusPending WebhookStatus = "pending"
	WebhookStatusSuccess WebhookStatus = "success"
	WebhookStatusFailed  WebhookStatus = "failed"
)

// Webhook is a project-scoped inbound webhook that triggers a service action.
type Webhook struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ProjectID string  `gorm:"not null;index" json:"project_id"`
	Project   Project `gorm:"foreignKey:ProjectID" json:"-"`
	CreatedBy string  `gorm:"not null" json:"created_by"`

	Name   string `gorm:"not null"              json:"name"`
	Active bool   `gorm:"not null;default:true" json:"active"`

	// HMAC secret — AES-256-GCM encrypted, never returned via API.
	Secret   string `gorm:"not null"                   json:"-"`
	Branch   string `                                  json:"branch"`
	Provider string `gorm:"not null;default:'generic'" json:"provider"`

	TriggerType WebhookTriggerType `gorm:"not null" json:"trigger_type"`

	// Redeploy trigger
	ServiceID *string `gorm:"index" json:"service_id,omitempty"`

	// Deploy trigger
	GitIntegrationID *string `gorm:"index"     json:"git_integration_id,omitempty"`
	RepoURL          string  `                 json:"repo_url,omitempty"`
	TemplateSlug     string  `                 json:"template_slug,omitempty"`
	FieldOverrides   string  `gorm:"type:text" json:"field_overrides,omitempty"`

	LastTriggeredAt *time.Time    `json:"last_triggered_at,omitempty"`
	LastStatus      WebhookStatus `json:"last_status,omitempty"`
	LastError       string        `json:"last_error,omitempty"`
	TriggerCount    int64         `gorm:"default:0" json:"trigger_count"`
}

// WebhookDelivery is an append-only log of every inbound webhook request.
type WebhookDelivery struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	WebhookID string    `gorm:"not null;index" json:"webhook_id"`

	Provider  string `json:"provider"`
	EventType string `json:"event_type"`
	Branch    string `json:"branch"`
	Commit    string `json:"commit"`
	CommitMsg string `json:"commit_msg"`
	PushedBy  string `json:"pushed_by"`
	RepoURL   string `json:"repo_url"`

	Status   WebhookStatus `json:"status"`
	ErrorMsg string        `json:"error_msg,omitempty"`
	JobID    string        `json:"job_id,omitempty"`
	Duration int64         `json:"duration_ms"`
}
