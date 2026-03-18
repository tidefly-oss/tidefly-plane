package models

import "time"

// Stack represents a deployed Compose or Dockerfile stack.
type Stack struct {
	ID         string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name       string    `gorm:"not null;size:255"                              json:"name"`
	Source     string    `gorm:"not null;size:64"                               json:"source"`
	RawContent string    `gorm:"type:text"                                      json:"raw_content,omitempty"`
	ProjectID  *string   `gorm:"type:uuid;index"                                json:"project_id,omitempty"`
	CreatedAt  time.Time `                                                      json:"created_at"`
}
