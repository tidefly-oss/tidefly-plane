package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/git/mapper"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
)

type SetSharesInput struct {
	ID   string `path:"id"`
	Body struct {
		ProjectIDs []string `json:"project_ids"`
	}
}
type SetSharesOutput struct {
	Body mapper.IntegrationResponse
}

func (h *Handler) SetShares(ctx context.Context, input *SetSharesInput) (*SetSharesOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.RequireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	txErr := h.integration.SetShares(m.ID, input.Body.ProjectIDs)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditGitRepoLink,
			ResourceID: m.ID,
			Success:    txErr == nil,
			Details: fmt.Sprintf(
				"integration=%s projects=%d [%s]",
				m.Name, len(input.Body.ProjectIDs),
				strings.Join(input.Body.ProjectIDs, ","),
			),
		},
	)
	if txErr != nil {
		return nil, fmt.Errorf("update shares: %w", txErr)
	}
	h.integration.Reload(m)
	return &SetSharesOutput{Body: mapper.ToIntegrationResponse(m, user.ID)}, nil
}
