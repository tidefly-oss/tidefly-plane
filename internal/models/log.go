package models

import "time"

// AppLog stores structured application log entries (written to DB + SSE stream).
// INFO level is intentionally never written to DB to avoid spam.
type AppLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Level       string    `gorm:"size:10;index"            json:"level"`
	Message     string    `gorm:"size:1000"                json:"message"`
	Component   string    `gorm:"size:100;index"           json:"component"`
	Error       string    `gorm:"size:2000"                json:"error"`
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
