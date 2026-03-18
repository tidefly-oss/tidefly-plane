package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Project is the central project model shared across the entire application.
// The handler logic lives in internal/api/v1/handlers/projects/handler.go.
type Project struct {
	ID          string    `gorm:"primaryKey;size:36"           json:"id"`
	Name        string    `gorm:"size:255;not null;uniqueIndex" json:"name"`
	Description string    `gorm:"size:1000"                    json:"description"`
	Color       string    `gorm:"size:7;default:'#6366f1'"     json:"color"`
	NetworkName string    `gorm:"size:255;uniqueIndex"         json:"network_name"`
	CreatedAt   time.Time `                                    json:"created_at"`
	UpdatedAt   time.Time `                                    json:"updated_at"`

	// Relations
	Members []ProjectMember `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"members,omitempty"`
}

func (p *Project) BeforeCreate(*gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	if p.NetworkName == "" {
		p.NetworkName = "tidefly_" + p.Name
	}
	return nil
}
