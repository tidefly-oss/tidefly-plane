package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type Handler struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

// ── ListAppLogs ───────────────────────────────────────────────────────────────

type ListAppLogsInput struct {
	Limit     int    `query:"limit" minimum:"1" maximum:"1000" default:"100"`
	Offset    int    `query:"offset" minimum:"0" default:"0"`
	Level     string `query:"level,omitempty"`
	Component string `query:"component,omitempty"`
}
type ListAppLogsOutput struct {
	Body struct {
		Logs   []models.AppLog `json:"logs"`
		Total  int64           `json:"total"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}
}

func (h *Handler) ListAppLogs(_ context.Context, input *ListAppLogsInput) (*ListAppLogsOutput, error) {
	if input.Limit <= 0 || input.Limit > 1000 {
		input.Limit = 100
	}
	query := h.db.Model(&models.AppLog{}).Order("created_at DESC").Limit(input.Limit).Offset(input.Offset)
	if input.Level != "" {
		query = query.Where("level = ?", input.Level)
	}
	if input.Component != "" {
		query = query.Where("component = ?", input.Component)
	}
	var appLogs []models.AppLog
	if err := query.Find(&appLogs).Error; err != nil {
		return nil, fmt.Errorf("list app logs: %w", err)
	}
	var total int64
	h.db.Model(&models.AppLog{}).Count(&total)

	out := &ListAppLogsOutput{}
	out.Body.Logs = appLogs
	out.Body.Total = total
	out.Body.Limit = input.Limit
	out.Body.Offset = input.Offset
	return out, nil
}

// ── ListAuditLogs ─────────────────────────────────────────────────────────────

type ListAuditLogsInput struct {
	Limit  int    `query:"limit" minimum:"1" maximum:"1000" default:"100"`
	Offset int    `query:"offset" minimum:"0" default:"0"`
	UserID string `query:"user_id,omitempty"`
	Action string `query:"action,omitempty"`
}
type ListAuditLogsOutput struct {
	Body struct {
		Logs   []models.AuditLog `json:"logs"`
		Total  int64             `json:"total"`
		Limit  int               `json:"limit"`
		Offset int               `json:"offset"`
	}
}

func (h *Handler) ListAuditLogs(_ context.Context, input *ListAuditLogsInput) (*ListAuditLogsOutput, error) {
	if input.Limit <= 0 || input.Limit > 1000 {
		input.Limit = 100
	}
	query := h.db.Model(&models.AuditLog{}).Order("created_at DESC").Limit(input.Limit).Offset(input.Offset)
	if input.UserID != "" {
		query = query.Where("user_id = ?", input.UserID)
	}
	if input.Action != "" {
		query = query.Where("action = ?", input.Action)
	}
	var auditLogs []models.AuditLog
	if err := query.Find(&auditLogs).Error; err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	var total int64
	h.db.Model(&models.AuditLog{}).Count(&total)

	out := &ListAuditLogsOutput{}
	out.Body.Logs = auditLogs
	out.Body.Total = total
	out.Body.Limit = input.Limit
	out.Body.Offset = input.Offset
	return out, nil
}

// ── StreamAppLogs (bleibt Echo — SSE) ────────────────────────────────────────

func (h *Handler) StreamAppLogs(c *echo.Context) error {
	level := c.QueryParam("level")
	component := c.QueryParam("component")

	resp := c.Response()
	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)

	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ctx := c.Request().Context()
	lastID := uint(0)
	var latest models.AppLog
	if err := h.db.Order("id DESC").First(&latest).Error; err == nil {
		lastID = latest.ID
	}

	heartbeat := time.NewTicker(15 * time.Second)
	poll := time.NewTicker(2 * time.Second)
	defer heartbeat.Stop()
	defer poll.Stop()

	sendEvent := func(event, data string) {
		_, _ = fmt.Fprintf(resp, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(resp, ": heartbeat\n\n")
			flusher.Flush()
		case <-poll.C:
			query := h.db.Model(&models.AppLog{}).Where("id > ?", lastID).Order("id ASC")
			if level != "" {
				query = query.Where("level = ?", level)
			}
			if component != "" {
				query = query.Where("component = ?", component)
			}
			var newLogs []models.AppLog
			if err := query.Find(&newLogs).Error; err != nil {
				continue
			}
			for _, entry := range newLogs {
				data, _ := json.Marshal(entry)
				sendEvent("log", string(data))
				if entry.ID > lastID {
					lastID = entry.ID
				}
			}
		}
	}
}
