package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceStatus string

const (
	ServiceStatusDeploying ServiceStatus = "deploying"
	ServiceStatusRunning   ServiceStatus = "running"
	ServiceStatusStopped   ServiceStatus = "stopped"
	ServiceStatusFailed    ServiceStatus = "failed"
)

// Service represents a deployed template instance.
type Service struct {
	ID           uuid.UUID     `gorm:"type:uuid;primaryKey"         json:"id"`
	Name         string        `gorm:"not null"                     json:"name"`
	TemplateSlug string        `gorm:"not null"                     json:"template_slug"`
	Version      string        `gorm:"not null"                     json:"version"`
	Status       ServiceStatus `gorm:"not null;default:'deploying'" json:"status"`
	ProjectID    string        `gorm:"not null;index"               json:"project_id"`
	CreatedAt    time.Time     `                                    json:"created_at"`
	UpdatedAt    time.Time     `                                    json:"updated_at"`

	Credentials []ServiceCredential `gorm:"foreignKey:ServiceID;constraint:OnDelete:CASCADE" json:"credentials,omitempty"`
}

func (s *Service) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// ServiceCredential stores a single credential for a service.
// The plaintext is only available at creation time — afterwards only the hash.
type ServiceCredential struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey"     json:"id"`
	ServiceID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"-"`
	Key              string     `gorm:"not null"                 json:"key"`
	Label            string     `gorm:"not null"                 json:"label"`
	Hash             string     `gorm:"not null"                 json:"-"`
	PlaintextShownAt *time.Time `                                json:"plaintext_shown_at,omitempty"`
	CreatedAt        time.Time  `                                json:"created_at"`
}

func (c *ServiceCredential) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
