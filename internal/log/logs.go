package log

import (
	"context"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type listAppLogsInput struct {
	Limit     int    `query:"limit"     minimum:"1" maximum:"1000" default:"100"`
	Offset    int    `query:"offset"    minimum:"0" default:"0"`
	Level     string `query:"level,omitempty"`
	Component string `query:"component,omitempty"`
}

type listAppLogsOutput struct {
	Body struct {
		Logs   []models.AppLog `json:"logs"`
		Total  int64           `json:"total"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}
}

type listAuditLogsInput struct {
	Limit  int    `query:"limit"  minimum:"1" maximum:"1000" default:"100"`
	Offset int    `query:"offset" minimum:"0" default:"0"`
	UserID string `query:"user_id,omitempty"`
	Action string `query:"action,omitempty"`
}

type listAuditLogsOutput struct {
	Body struct {
		Logs   []models.AuditLog `json:"logs"`
		Total  int64             `json:"total"`
		Limit  int               `json:"limit"`
		Offset int               `json:"offset"`
	}
}

func (h *Handler) listAppLogs(_ context.Context, input *listAppLogsInput) (*listAppLogsOutput, error) {
	if input.Limit <= 0 || input.Limit > 1000 {
		input.Limit = 100
	}
	result, err := h.store.listAppLogs(appLogQuery{
		Limit: input.Limit, Offset: input.Offset,
		Level: input.Level, Component: input.Component,
	})
	if err != nil {
		return nil, err
	}
	out := &listAppLogsOutput{}
	out.Body.Logs = result.Logs
	out.Body.Total = result.Total
	out.Body.Limit = input.Limit
	out.Body.Offset = input.Offset
	return out, nil
}

func (h *Handler) listAuditLogs(_ context.Context, input *listAuditLogsInput) (*listAuditLogsOutput, error) {
	if input.Limit <= 0 || input.Limit > 1000 {
		input.Limit = 100
	}
	result, err := h.store.listAuditLogs(auditLogQuery{
		Limit: input.Limit, Offset: input.Offset,
		UserID: input.UserID, Action: input.Action,
	})
	if err != nil {
		return nil, err
	}
	out := &listAuditLogsOutput{}
	out.Body.Logs = result.Logs
	out.Body.Total = result.Total
	out.Body.Limit = input.Limit
	out.Body.Offset = input.Offset
	return out, nil
}
