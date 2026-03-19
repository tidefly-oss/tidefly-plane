package http

import (
	"context"

	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type GetSettingsInput struct{}
type GetSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) GetSettings(_ context.Context, _ *GetSettingsInput) (*GetSettingsOutput, error) {
	s, err := h.settings.Get()
	if err != nil {
		return nil, err
	}
	return &GetSettingsOutput{Body: s}, nil
}

type UpdateSettingsInput struct {
	Body service.SettingsUpdateInput
}
type UpdateSettingsOutput struct {
	Body models.SystemSettings
}

func (h *Handler) UpdateSettings(ctx context.Context, input *UpdateSettingsInput) (*UpdateSettingsOutput, error) {
	s, err := h.settings.Update(input.Body)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:  logger.AuditAdminSettingsUpdate,
		Success: err == nil,
	})
	if err != nil {
		return nil, err
	}
	return &UpdateSettingsOutput{Body: s}, nil
}
