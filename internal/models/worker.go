package models

import (
	"time"

	"gorm.io/gorm"
)

type WorkerStatus string

const (
	WorkerStatusPending      WorkerStatus = "pending"
	WorkerStatusConnected    WorkerStatus = "connected"
	WorkerStatusDisconnected WorkerStatus = "disconnected"
	WorkerStatusRevoked      WorkerStatus = "revoked"
)

type WorkerNode struct {
	ID        string         `gorm:"type:varchar(64);primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Name        string `gorm:"type:varchar(128);not null" json:"name"`
	Description string `gorm:"type:varchar(512)" json:"description"`

	Status     WorkerStatus `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	LastSeenAt *time.Time   `json:"last_seen_at,omitempty"`
	LastSeenIP string       `gorm:"type:varchar(64)" json:"last_seen_ip,omitempty"`

	AgentVersion string `gorm:"type:varchar(32)" json:"agent_version,omitempty"`
	OS           string `gorm:"type:varchar(64)" json:"os,omitempty"`
	Arch         string `gorm:"type:varchar(32)" json:"arch,omitempty"`
	RuntimeType  string `gorm:"type:varchar(32)" json:"runtime_type,omitempty"`

	// Metrics — updated on every heartbeat
	CPUPercent     float64 `gorm:"type:numeric(5,2);default:0" json:"cpu_percent"`
	MemPercent     float64 `gorm:"type:numeric(5,2);default:0" json:"mem_percent"`
	ContainerCount int32   `gorm:"default:0" json:"container_count"`

	RegisteredByUserID string `gorm:"type:varchar(64);not null" json:"registered_by_user_id"`
}

func (*WorkerNode) TableName() string { return "worker_nodes" }

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

func (w *WorkerNode) UpdateHeartbeat(cpuPercent, memPercent float64, containerCount int32) {
	now := time.Now()
	w.LastSeenAt = &now
	w.CPUPercent = cpuPercent
	w.MemPercent = memPercent
	w.ContainerCount = containerCount
}
