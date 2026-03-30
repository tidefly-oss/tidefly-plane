package models

import "time"

// BackupConfig stores the global S3-compatible backups configuration.
// Only one record exists (id = 1).
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

// BackupRecord tracks completed backups jobs.
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
