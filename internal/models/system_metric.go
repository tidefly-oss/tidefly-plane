package models

import "time"

// SystemMetric stores a point-in-time snapshot of host resource usage.
// Collected every 60s by the metrics:collect background job.
type SystemMetric struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CPUPercent  float64   `gorm:"not null"                 json:"cpu_percent"`
	MemUsedMB   int64     `gorm:"not null"                 json:"mem_used_mb"`
	MemTotalMB  int64     `gorm:"not null"                 json:"mem_total_mb"`
	MemPercent  float64   `gorm:"not null"                 json:"mem_percent"`
	DiskUsedMB  int64     `gorm:"not null"                 json:"disk_used_mb"`
	DiskTotalMB int64     `gorm:"not null"                 json:"disk_total_mb"`
	DiskPercent float64   `gorm:"not null"                 json:"disk_percent"`
	CollectedAt time.Time `gorm:"not null;index"           json:"collected_at"`
}
