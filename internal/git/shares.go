package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type setSharesInput struct {
	ID   string `path:"id"`
	Body struct {
		ProjectIDs []string `json:"project_ids"`
	}
}

type setSharesOutput struct {
	Body integrationResponse
}

func (h *Handler) setShares(ctx context.Context, input *setSharesInput) (*setSharesOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.store.RequireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	txErr := h.store.SetShares(m.ID, input.Body.ProjectIDs)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditGitRepoLink,
		ResourceID: m.ID,
		Success:    txErr == nil,
		Details: fmt.Sprintf(
			"integration=%s projects=%d [%s]",
			m.Name, len(input.Body.ProjectIDs),
			strings.Join(input.Body.ProjectIDs, ","),
		),
	})
	if txErr != nil {
		return nil, fmt.Errorf("update shares: %w", txErr)
	}
	h.store.Reload(m)
	return &setSharesOutput{Body: toIntegrationResponse(m, user.ID)}, nil
}
