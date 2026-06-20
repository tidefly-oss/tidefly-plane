package log

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

type appLogQuery struct {
	Limit     int
	Offset    int
	Level     string
	Component string
}

type auditLogQuery struct {
	Limit  int
	Offset int
	UserID string
	Action string
}

type pagedAppLogs struct {
	Logs  []models.AppLog
	Total int64
}

type pagedAuditLogs struct {
	Logs  []models.AuditLog
	Total int64
}

func (s *Store) listAppLogs(q appLogQuery) (pagedAppLogs, error) {
	query := s.db.Model(&models.AppLog{}).Order("created_at DESC").Limit(q.Limit).Offset(q.Offset)
	if q.Level != "" {
		query = query.Where("level = ?", q.Level)
	}
	if q.Component != "" {
		query = query.Where("component = ?", q.Component)
	}
	var logs []models.AppLog
	if err := query.Find(&logs).Error; err != nil {
		return pagedAppLogs{}, fmt.Errorf("list app logs: %w", err)
	}
	var total int64
	s.db.Model(&models.AppLog{}).Count(&total)
	return pagedAppLogs{Logs: logs, Total: total}, nil
}

func (s *Store) listAuditLogs(q auditLogQuery) (pagedAuditLogs, error) {
	query := s.db.Model(&models.AuditLog{}).Order("created_at DESC").Limit(q.Limit).Offset(q.Offset)
	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}
	if q.Action != "" {
		query = query.Where("action = ?", q.Action)
	}
	var logs []models.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		return pagedAuditLogs{}, fmt.Errorf("list audit logs: %w", err)
	}
	var total int64
	s.db.Model(&models.AuditLog{}).Count(&total)
	return pagedAuditLogs{Logs: logs, Total: total}, nil
}

func (s *Store) latestAppLogID() uint {
	var latest models.AppLog
	if err := s.db.Order("id DESC").First(&latest).Error; err == nil {
		return latest.ID
	}
	return 0
}

func (s *Store) pollAppLogs(lastID uint, level, component string) ([]models.AppLog, error) {
	query := s.db.Model(&models.AppLog{}).Where("id > ?", lastID).Order("id ASC")
	if level != "" {
		query = query.Where("level = ?", level)
	}
	if component != "" {
		query = query.Where("component = ?", component)
	}
	var logs []models.AppLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}
