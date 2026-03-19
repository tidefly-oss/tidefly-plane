package http

import (
	"context"

	logsvc "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/logs/service"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

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

func (h *Handler) ListAppLogs(_ context.Context, input *ListAppLogsInput) (*ListAppLogsOutput, error) {
	if input.Limit <= 0 || input.Limit > 1000 {
		input.Limit = 100
	}
	result, err := h.logs.ListAppLogs(
		logsvc.AppLogQuery{
			Limit: input.Limit, Offset: input.Offset,
			Level: input.Level, Component: input.Component,
		},
	)
	if err != nil {
		return nil, err
	}
	out := &ListAppLogsOutput{}
	out.Body.Logs = result.Logs
	out.Body.Total = result.Total
	out.Body.Limit = input.Limit
	out.Body.Offset = input.Offset
	return out, nil
}

func (h *Handler) ListAuditLogs(_ context.Context, input *ListAuditLogsInput) (*ListAuditLogsOutput, error) {
	if input.Limit <= 0 || input.Limit > 1000 {
		input.Limit = 100
	}
	result, err := h.logs.ListAuditLogs(
		logsvc.AuditLogQuery{
			Limit: input.Limit, Offset: input.Offset,
			UserID: input.UserID, Action: input.Action,
		},
	)
	if err != nil {
		return nil, err
	}
	out := &ListAuditLogsOutput{}
	out.Body.Logs = result.Logs
	out.Body.Total = result.Total
	out.Body.Limit = input.Limit
	out.Body.Offset = input.Offset
	return out, nil
}
