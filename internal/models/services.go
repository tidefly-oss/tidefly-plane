package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceStatus string

const (
	ServiceStatusDeploying  ServiceStatus = "deploying"
	ServiceStatusRunning    ServiceStatus = "running"
	ServiceStatusStopped    ServiceStatus = "stopped"
	ServiceStatusRestarting ServiceStatus = "restarting"
	ServiceStatusFailed     ServiceStatus = "failed"
)

// UpdateSource identifies what triggered the update_available flag.
type UpdateSource string

const (
	UpdateSourceRegistry UpdateSource = "registry" // OCI digest check
	UpdateSourceGit      UpdateSource = "git"      // Git push webhook
	UpdateSourceTemplate UpdateSource = "template" // tidefly-templates webhook
)

// Service is the core managed unit in Tidefly.
// ManifestJSON is the desired state — the Reconciler compares it against
// actual runtime state and acts on any delta.
type Service struct {
	ID           uuid.UUID     `gorm:"type:uuid;primaryKey"         json:"id"`
	Name         string        `gorm:"not null;uniqueIndex"         json:"name"`
	TemplateSlug string        `gorm:"not null;default:''"          json:"template_slug"`
	Version      string        `gorm:"not null;default:''"          json:"version"`
	Status       ServiceStatus `gorm:"not null;default:'deploying'" json:"status"`
	ProjectID    string        `gorm:"not null;index;default:''"    json:"project_id"`

	// WorkerID is set when the service runs on a remote worker node.
	// Empty = runs on the local plane node.
	WorkerID string `gorm:"type:varchar(64);default:''" json:"worker_id,omitempty"`

	// ManifestJSON is the source of truth for desired state.
	// Stored as JSON internally, defined by Contributors as JSON templates.
	ManifestService bool   `gorm:"not null;default:false" json:"manifest_service"`
	ManifestJSON    string `gorm:"type:text;default:''"   json:"manifest_json,omitempty"`

	// Deployment metadata
	PublicURL      string `gorm:"type:text;default:''"          json:"public_url,omitempty"`
	LastError      string `gorm:"type:text;default:''"          json:"last_error,omitempty"`
	ActiveSlotName string `gorm:"column:active_slot;default:''" json:"-"`

	// ── OCI Image Tracking ────────────────────────────────────────────────
	// DeployedDigest is the OCI digest of the image currently running.
	// Set by the Reconciler after a successful deploy.
	// Format: sha256:<64-hex>
	DeployedDigest string `gorm:"type:varchar(128);default:''" json:"deployed_digest,omitempty"`

	// RemoteDigest is the latest digest fetched from the OCI registry.
	// The Reconciler compares RemoteDigest vs DeployedDigest to detect updates.
	RemoteDigest string `gorm:"type:varchar(128);default:''" json:"remote_digest,omitempty"`

	// UpdateAvailable is true when RemoteDigest != DeployedDigest
	// or when a Git/template webhook fired.
	UpdateAvailable bool         `gorm:"not null;default:false"       json:"update_available"`
	UpdateSource    UpdateSource `gorm:"type:varchar(32);default:''"  json:"update_source,omitempty"`

	// UpdateCheckedAt is when the Reconciler last polled the OCI registry.
	// Indexed together with Status for efficient Reconciler queries.
	UpdateCheckedAt *time.Time `gorm:"index:idx_service_update_check,priority:2" json:"update_checked_at,omitempty"`

	CreatedAt   time.Time           `gorm:"index:idx_service_update_check,priority:1" json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	Credentials []ServiceCredential `gorm:"foreignKey:ServiceID;constraint:OnDelete:CASCADE" json:"credentials,omitempty"`
}

func (s *Service) BeforeCreate(_ *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (s *Service) IsManifestService() bool { return s.ManifestService }
func (s *Service) ActiveSlot() string      { return s.ActiveSlotName }

// HasImageDrift returns true when the running image differs from the registry.
// Used by the Reconciler to decide whether to trigger a blue-green or rolling update.
func (s *Service) HasImageDrift() bool {
	return s.RemoteDigest != "" &&
		s.DeployedDigest != "" &&
		s.RemoteDigest != s.DeployedDigest
}

// ServiceCredential stores a hashed credential for a service.
// Plaintext is only available at creation time.
type ServiceCredential struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"     json:"id"`
	ServiceID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"-"`
	Key              string     `gorm:"not null"                 json:"key"`
	Label            string     `gorm:"not null"                 json:"label"`
	Hash             string     `gorm:"not null"                 json:"-"`
	PlaintextShownAt *time.Time `                                json:"plaintext_shown_at,omitempty"`
	CreatedAt        time.Time  `                                json:"created_at"`
}

func (c *ServiceCredential) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// Stack represents a deployed Compose or Dockerfile stack (multi-service).
type Stack struct {
	ID         string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name       string    `gorm:"not null;size:255"                              json:"name"`
	Source     string    `gorm:"not null;size:64"                               json:"source"`
	RawContent string    `gorm:"type:text"                                      json:"raw_content,omitempty"`
	ProjectID  *string   `gorm:"type:uuid;index"                                json:"project_id,omitempty"`
	CreatedAt  time.Time `                                                      json:"created_at"`
}
