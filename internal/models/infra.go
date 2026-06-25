package models

import (
	"time"

	"gorm.io/gorm"
)

// ── Worker Nodes ──────────────────────────────────────────────────────────────

type WorkerStatus string

const (
	WorkerStatusPending      WorkerStatus = "pending"
	WorkerStatusConnected    WorkerStatus = "connected"
	WorkerStatusDisconnected WorkerStatus = "disconnected"
	WorkerStatusRevoked      WorkerStatus = "revoked"
)

// WorkerNode is a remote agent that runs containers on behalf of the plane.
type WorkerNode struct {
	ID        string         `gorm:"type:varchar(64);primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Name        string `gorm:"type:varchar(128);not null" json:"name"`
	Description string `gorm:"type:varchar(512)"          json:"description"`

	Status     WorkerStatus `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	LastSeenAt *time.Time   `json:"last_seen_at,omitempty"`
	LastSeenIP string       `gorm:"type:varchar(64)" json:"last_seen_ip,omitempty"`

	AgentVersion string `gorm:"type:varchar(32)" json:"agent_version,omitempty"`
	OS           string `gorm:"type:varchar(64)" json:"os,omitempty"`
	Arch         string `gorm:"type:varchar(32)" json:"arch,omitempty"`
	RuntimeType  string `gorm:"type:varchar(32)" json:"runtime_type,omitempty"`

	CPUPercent     float64 `gorm:"type:numeric(5,2);default:0" json:"cpu_percent"`
	MemPercent     float64 `gorm:"type:numeric(5,2);default:0" json:"mem_percent"`
	ContainerCount int32   `gorm:"default:0"                   json:"container_count"`

	RegisteredByUserID string `gorm:"type:varchar(64);not null" json:"registered_by_user_id"`
}

func (*WorkerNode) TableName() string { return "worker_nodes" }

func (w *WorkerNode) IsActive() bool { return w.Status == WorkerStatusConnected }

func (w *WorkerNode) MarkConnected(ip, version string) {
	w.Status = WorkerStatusConnected
	w.LastSeenAt = new(time.Now())
	w.LastSeenIP = ip
	w.AgentVersion = version
}

func (w *WorkerNode) MarkDisconnected() {
	w.Status = WorkerStatusDisconnected
}

func (w *WorkerNode) UpdateHeartbeat(cpuPercent, memPercent float64, containerCount int32) {
	w.LastSeenAt = new(time.Now())
	w.CPUPercent = cpuPercent
	w.MemPercent = memPercent
	w.ContainerCount = containerCount
}

// ── Git Integrations ──────────────────────────────────────────────────────────

// GitIntegration stores a connected Git provider with encrypted credentials.
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

	Shares []GitIntegrationShare `gorm:"foreignKey:IntegrationID" json:"-"`
}

// GitIntegrationShare allows sharing a Git integration with a project.
type GitIntegrationShare struct {
	IntegrationID string    `gorm:"type:uuid;not null;index:idx_git_share,unique" json:"integration_id"`
	ProjectID     string    `gorm:"type:uuid;not null;index:idx_git_share,unique" json:"project_id"`
	CreatedAt     time.Time `                                                      json:"created_at"`
}

// ── Backups ───────────────────────────────────────────────────────────────────

// BackupConfig stores the global S3-compatible backup configuration (singleton).
type BackupConfig struct {
	ID        uint      `gorm:"primaryKey"          json:"id"`
	Endpoint  string    `gorm:"not null"            json:"endpoint"`
	Bucket    string    `gorm:"not null"            json:"bucket"`
	Region    string    `gorm:"default:'us-east-1'" json:"region"`
	AccessKey string    `gorm:"not null"            json:"-"`
	SecretKey string    `gorm:"not null"            json:"-"`
	UseSSL    bool      `gorm:"default:true"        json:"use_ssl"`
	PathStyle bool      `gorm:"default:false"       json:"path_style"`
	Prefix    string    `gorm:"default:'backups'"   json:"prefix"`
	CreatedAt time.Time `                           json:"created_at"`
	UpdatedAt time.Time `                           json:"updated_at"`
}

// BackupRecord tracks completed backup jobs.
type BackupRecord struct {
	ID        uint      `gorm:"primaryKey"   json:"id"`
	ProjectID string    `gorm:"index"        json:"project_id"`
	ServiceID string    `gorm:"index"        json:"service_id"`
	Type      string    `gorm:"not null"     json:"type"`
	S3Key     string    `gorm:"not null"     json:"s3_key"`
	SizeBytes int64     `                    json:"size_bytes"`
	Status    string    `gorm:"not null"     json:"status"`
	Error     string    `                    json:"error,omitempty"`
	CreatedAt time.Time `                    json:"created_at"`
}

// ── Certificates ──────────────────────────────────────────────────────────────

// CertificateAuthority is the Tidefly internal CA (singleton).
// CertPEM and KeyPEM are AES-256-GCM encrypted.
type CertificateAuthority struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	CertPEM   string    `gorm:"type:text;not null"         json:"-"`
	KeyPEM    string    `gorm:"type:text;not null"         json:"-"`
	Subject   string    `gorm:"type:varchar(255);not null" json:"subject"`
	NotBefore time.Time `gorm:"not null"                   json:"not_before"`
	NotAfter  time.Time `gorm:"not null"                   json:"not_after"`
	Serial    string    `gorm:"type:varchar(64);not null"  json:"serial"`
}

func (ca *CertificateAuthority) TableName() string { return "certificate_authorities" }

func (ca *CertificateAuthority) IsExpiringSoon(within time.Duration) bool {
	return time.Until(ca.NotAfter) < within
}

// IssuedCertificate tracks every cert the CA has signed.
type IssuedCertificate struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	OwnerType string `gorm:"type:varchar(32);not null;index" json:"owner_type"` // "worker" | "plane"
	OwnerID   string `gorm:"type:varchar(64);not null;index" json:"owner_id"`

	CertPEM   string    `gorm:"type:text;not null"                   json:"-"`
	KeyPEM    string    `gorm:"type:text;not null"                   json:"-"`
	Subject   string    `gorm:"type:varchar(255);not null"           json:"subject"`
	NotBefore time.Time `gorm:"not null"                             json:"not_before"`
	NotAfter  time.Time `gorm:"not null"                             json:"not_after"`
	Serial    string    `gorm:"type:varchar(64);not null;uniqueIndex" json:"serial"`

	Revoked   bool       `gorm:"default:false;not null" json:"revoked"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	RevokedBy string     `gorm:"type:varchar(64)" json:"revoked_by,omitempty"`

	RenewedFromID *uint `gorm:"index" json:"renewed_from_id,omitempty"`
	RenewedToID   *uint `gorm:"index" json:"renewed_to_id,omitempty"`
}

func (c *IssuedCertificate) TableName() string { return "issued_certificates" }

func (c *IssuedCertificate) IsExpiringSoon(within time.Duration) bool {
	return !c.Revoked && time.Until(c.NotAfter) < within
}

func (c *IssuedCertificate) IsValid() bool {
	now := time.Now()
	return !c.Revoked && now.After(c.NotBefore) && now.Before(c.NotAfter)
}

// WorkerRegistrationToken is a one-time token for worker onboarding.
type WorkerRegistrationToken struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Token     string     `gorm:"type:varchar(128);not null;uniqueIndex" json:"token"`
	ExpiresAt time.Time  `gorm:"not null"                               json:"expires_at"`
	Used      bool       `gorm:"default:false;not null"                 json:"used"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	Label     string     `gorm:"type:varchar(128)" json:"label"`
	WorkerID  *string    `gorm:"type:varchar(64);index" json:"worker_id,omitempty"`

	CreatedByUserID string `gorm:"type:varchar(64);not null" json:"created_by_user_id"`
}

func (t *WorkerRegistrationToken) TableName() string { return "worker_registration_tokens" }

func (t *WorkerRegistrationToken) IsValid() bool {
	return !t.Used && time.Now().Before(t.ExpiresAt)
}
