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

// Service represents a deployed manifest-based service managed by Tidefly.
// ManifestJSON is the source of truth for desired state — it is re-resolved
// on every update/redeploy without requiring the user to re-submit.
type Service struct {
	ID           uuid.UUID     `gorm:"type:uuid;primaryKey"         json:"id"`
	Name         string        `gorm:"not null"                     json:"name"`
	TemplateSlug string        `gorm:"not null;default:''"          json:"template_slug"`
	Version      string        `gorm:"not null;default:''"          json:"version"`
	Status       ServiceStatus `gorm:"not null;default:'deploying'" json:"status"`
	ProjectID    string        `gorm:"not null;index;default:''"    json:"project_id"`
	WorkerID     string        `gorm:"type:varchar(64);default:''"  json:"worker_id,omitempty"`

	// Manifest-based services
	ManifestService bool   `gorm:"not null;default:false" json:"manifest_service"`
	ManifestJSON    string `gorm:"type:text;default:''"   json:"manifest_json,omitempty"`

	// Deployment metadata
	PublicURL      string `gorm:"type:text;default:''" json:"public_url,omitempty"`
	LastError      string `gorm:"type:text;default:''" json:"last_error,omitempty"`
	ActiveSlotName string `gorm:"column:active_slot;default:''" json:"-"`

	CreatedAt   time.Time           `json:"created_at"`
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

func (s *Service) ActiveSlot() string { return s.ActiveSlotName }

// ServiceCredential stores a single credential for a service.
// Plaintext is only available at creation time — afterwards only the hash.
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
