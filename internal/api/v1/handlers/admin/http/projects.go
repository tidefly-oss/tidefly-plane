package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/admin/mapper"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
)

type SetProjectMembersInput struct {
	ID   string `path:"id" doc:"User ID"`
	Body struct {
		ProjectIDs []string `json:"project_ids" doc:"Project IDs to assign"`
	}
}
type SetProjectMembersOutput struct {
	Body mapper.AdminUserResponse
}

func (h *Handler) SetProjectMembers(ctx context.Context, input *SetProjectMembersInput) (*SetProjectMembersOutput, error) {
	if input.Body.ProjectIDs == nil {
		input.Body.ProjectIDs = []string{}
	}

	u, err := h.users.SetProjectMembers(input.ID, input.Body.ProjectIDs)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditAdminUserProjectsUpdate,
		ResourceID: input.ID,
		Success:    err == nil,
		Details: fmt.Sprintf(
			"email=%s projects=%d [%s]",
			u.Email, len(input.Body.ProjectIDs),
			strings.Join(input.Body.ProjectIDs, ","),
		),
	})
	if err != nil {
		if err.Error() == "invalid project ids" {
			return nil, huma400("one or more project IDs are invalid")
		}
		return nil, huma404("user not found")
	}

	return &SetProjectMembersOutput{Body: mapper.ToAdminUserResponse(&u)}, nil
}
