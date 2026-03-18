package models

import (
	"time"

	"gorm.io/gorm"
)

// GitIntegration stores a connected Git provider with encrypted credentials.
// Owned by a single user — never exposed across users.
// Can be shared with projects via GitIntegrationShare.
type GitIntegration struct {
	ID              string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID          string    `gorm:"type:uuid;not null;index"                       json:"user_id"`
	Name            string    `gorm:"not null;size:255"                              json:"name"`
	Provider        string    `gorm:"not null;size:64;index"                         json:"provider"`
	BaseURL         string    `gorm:"size:512"                                       json:"base_url,omitempty"`
	AuthType        string    `gorm:"not null;size:32"                               json:"auth_type"`
	SecretEncrypted string    `gorm:"not null;type:text"                             json:"-"`
	CreatedAt       time.Time `                                                      json:"created_at"`
	UpdatedAt       time.Time `                                                      json:"updated_at"`

	// Associations
	Shares []GitIntegrationShare `gorm:"foreignKey:IntegrationID" json:"-"`
}

func (g *GitIntegration) BeforeCreate(tx *gorm.DB) error {
	return nil
}

// GitIntegrationShare allows an owner to share a Git integration with a project.
// Members of the shared project can use the integration (read repos/branches, deploy)
// but cannot see credentials or modify the integration itself.
type GitIntegrationShare struct {
	IntegrationID string    `gorm:"type:uuid;not null;index:idx_git_share,unique" json:"integration_id"`
	ProjectID     string    `gorm:"type:uuid;not null;index:idx_git_share,unique" json:"project_id"`
	CreatedAt     time.Time `                                                      json:"created_at"`
}
