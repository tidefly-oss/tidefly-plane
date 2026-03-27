package models

import (
	"time"

	"gorm.io/gorm"
)

// WorkerStatus represents the current connection state of a worker agent.
type WorkerStatus string

const (
	WorkerStatusPending      WorkerStatus = "pending"      // registered, never connected
	WorkerStatusConnected    WorkerStatus = "connected"    // gRPC stream active
	WorkerStatusDisconnected WorkerStatus = "disconnected" // stream lost / graceful disconnect
	WorkerStatusRevoked      WorkerStatus = "revoked"      // manually disabled by admin
)

// WorkerNode represents a remote server running the tidefly-agent binary.
// The Plane manages workers; each worker connects back via gRPC mTLS.
type WorkerNode struct {
	ID        string         `gorm:"type:varchar(64);primaryKey" json:"id"` // UUID, set by worker on first register
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Human-readable name set during registration (e.g. "hetzner-fsn1-01")
	Name string `gorm:"type:varchar(128);not null" json:"name"`

	// Optional description
	Description string `gorm:"type:varchar(512)" json:"description"`

	// Connection state
	Status     WorkerStatus `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	LastSeenAt *time.Time   `json:"last_seen_at,omitempty"`

	// The IP the worker last connected from (informational)
	LastSeenIP string `gorm:"type:varchar(64)" json:"last_seen_ip,omitempty"`

	// Agent version reported on connect
	AgentVersion string `gorm:"type:varchar(32)" json:"agent_version,omitempty"`

	// OS / architecture info reported on connect
	OS   string `gorm:"type:varchar(64)" json:"os,omitempty"`
	Arch string `gorm:"type:varchar(32)" json:"arch,omitempty"`

	// Runtime on the worker (docker/podman)
	RuntimeType string `gorm:"type:varchar(32)" json:"runtime_type,omitempty"`

	// Who registered this worker (UUID string, matches auth.Claims.UserID)
	RegisteredByUserID string `gorm:"type:varchar(64);not null" json:"registered_by_user_id"`

	// Links
	IssuedCertificates []IssuedCertificate `gorm:"foreignKey:OwnerID;references:ID" json:"-"`
}

func (WorkerNode) TableName() string { return "worker_nodes" }

func (w *WorkerNode) IsActive() bool {
	return w.Status == WorkerStatusConnected
}

func (w *WorkerNode) MarkConnected(ip string, version string) {
	now := time.Now()
	w.Status = WorkerStatusConnected
	w.LastSeenAt = &now
	w.LastSeenIP = ip
	w.AgentVersion = version
}

func (w *WorkerNode) MarkDisconnected() {
	w.Status = WorkerStatusDisconnected
}
