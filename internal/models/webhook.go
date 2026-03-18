package models

import (
	"time"
)

// WebhookTriggerType defines what action a webhooks performs on trigger.
type WebhookTriggerType string

const (
	// WebhookTriggerRedeploy pulls the latest image and recreates the existing service.
	WebhookTriggerRedeploy WebhookTriggerType = "redeploy"
	// WebhookTriggerDeploy runs a full deploy from a template + Git source.
	WebhookTriggerDeploy WebhookTriggerType = "deploy"
)

// WebhookStatus is the result of the last trigger.
type WebhookStatus string

const (
	WebhookStatusPending WebhookStatus = "pending"
	WebhookStatusSuccess WebhookStatus = "success"
	WebhookStatusFailed  WebhookStatus = "failed"
)

// Webhook represents a project-scoped inbound webhooks that triggers a deploy.
//
// Public receiver:  POST /webhooks/:id          (no auth, HMAC verified)
// Management API:   /api/v1/projects/:pid/webhooks
type Webhook struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Ownership
	ProjectID string  `gorm:"not null;index" json:"project_id"`
	Project   Project `gorm:"foreignKey:ProjectID" json:"-"`
	CreatedBy string  `gorm:"not null" json:"created_by"` // user ID

	// Identity
	Name   string `gorm:"not null" json:"name"`
	Active bool   `gorm:"not null;default:true" json:"active"`

	// HMAC secret — AES-256-GCM encrypted at rest, never returned via API.
	// Providers send X-Hub-Signature-256: sha256=<hmac(secret, body)>
	Secret string `gorm:"not null" json:"-"`

	// Branch filter — empty string means all branches.
	// Supports exact match ("main") or wildcard ("*").
	Branch string `json:"branch"`

	// Provider hint for signature header selection.
	// "github" | "gitlab" | "gitea" | "bitbucket" | "generic"
	Provider string `gorm:"not null;default:'generic'" json:"provider"`

	// Trigger type
	TriggerType WebhookTriggerType `gorm:"not null" json:"trigger_type"`

	// ── Redeploy trigger ───────────────────────────────────────────────────
	// Pull latest image and recreate this service container.
	ServiceID *string `gorm:"index" json:"service_id,omitempty"`

	// ── Deploy trigger ─────────────────────────────────────────────────────
	// Run a full deploy from Git source + template.
	GitIntegrationID *string `gorm:"index" json:"git_integration_id,omitempty"`
	RepoURL          string  `json:"repo_url,omitempty"`
	TemplateSlug     string  `json:"template_slug,omitempty"`
	// FieldOverrides are merged into the template fields on deploy.
	// Stored as JSON: {"PORT": "3000", "IMAGE_TAG": "{{.branch}}"}
	// Supports {{.branch}}, {{.commit}}, {{.tag}} placeholders.
	FieldOverrides string `gorm:"type:text" json:"field_overrides,omitempty"`

	// ── Last trigger state ─────────────────────────────────────────────────
	LastTriggeredAt *time.Time    `json:"last_triggered_at,omitempty"`
	LastStatus      WebhookStatus `json:"last_status,omitempty"`
	LastError       string        `json:"last_error,omitempty"`
	TriggerCount    int64         `gorm:"default:0" json:"trigger_count"`
}

// WebhookDelivery is an append-only log of every inbound webhooks request.
type WebhookDelivery struct {
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	WebhookID string    `gorm:"not null;index" json:"webhook_id"`

	// Request snapshot
	Provider  string `json:"provider"`
	EventType string `json:"event_type"` // e.g. "push", "tag"
	Branch    string `json:"branch"`
	Commit    string `json:"commit"`
	CommitMsg string `json:"commit_msg"`
	PushedBy  string `json:"pushed_by"`
	RepoURL   string `json:"repo_url"`

	// Result
	Status   WebhookStatus `json:"status"`
	ErrorMsg string        `json:"error_msg,omitempty"`
	JobID    string        `json:"job_id,omitempty"` // asynq task ID
	Duration int64         `json:"duration_ms"`
}
