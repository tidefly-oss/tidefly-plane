package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type LogService struct {
	db *gorm.DB
}

func New(db *gorm.DB) *LogService {
	return &LogService{db: db}
}

type AppLogQuery struct {
	Limit     int
	Offset    int
	Level     string
	Component string
}

type AuditLogQuery struct {
	Limit  int
	Offset int
	UserID string
	Action string
}

type PagedAppLogs struct {
	Logs  []models.AppLog
	Total int64
}

type PagedAuditLogs struct {
	Logs  []models.AuditLog
	Total int64
}

func (s *LogService) ListAppLogs(q AppLogQuery) (PagedAppLogs, error) {
	query := s.db.Model(&models.AppLog{}).Order("created_at DESC").Limit(q.Limit).Offset(q.Offset)
	if q.Level != "" {
		query = query.Where("level = ?", q.Level)
	}
	if q.Component != "" {
		query = query.Where("component = ?", q.Component)
	}
	var logs []models.AppLog
	if err := query.Find(&logs).Error; err != nil {
		return PagedAppLogs{}, fmt.Errorf("list app logs: %w", err)
	}
	var total int64
	s.db.Model(&models.AppLog{}).Count(&total)
	return PagedAppLogs{Logs: logs, Total: total}, nil
}

func (s *LogService) ListAuditLogs(q AuditLogQuery) (PagedAuditLogs, error) {
	query := s.db.Model(&models.AuditLog{}).Order("created_at DESC").Limit(q.Limit).Offset(q.Offset)
	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}
	if q.Action != "" {
		query = query.Where("action = ?", q.Action)
	}
	var logs []models.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		return PagedAuditLogs{}, fmt.Errorf("list audit logs: %w", err)
	}
	var total int64
	s.db.Model(&models.AuditLog{}).Count(&total)
	return PagedAuditLogs{Logs: logs, Total: total}, nil
}

func (s *LogService) LatestAppLogID() uint {
	var latest models.AppLog
	if err := s.db.Order("id DESC").First(&latest).Error; err == nil {
		return latest.ID
	}
	return 0
}

func (s *LogService) PollAppLogs(lastID uint, level, component string) ([]models.AppLog, error) {
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
