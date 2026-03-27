package http

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/git/mapper"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/services/git/types"
)

type ListInput struct{}
type ListOutput struct {
	Body []mapper.IntegrationResponse
}

type CreateInput struct {
	Body struct {
		Name     string `json:"name"               minLength:"1" maxLength:"255"`
		Provider string `json:"provider"           enum:"github,gitlab,gitea-forgejo,bitbucket"`
		BaseURL  string `json:"base_url,omitempty"`
		Token    string `json:"token"              minLength:"1"`
		Username string `json:"username,omitempty" maxLength:"255"`
	}
}
type CreateOutput struct {
	Body mapper.IntegrationResponse
}

type GetInput struct {
	ID string `path:"id"`
}
type GetOutput struct {
	Body mapper.IntegrationResponse
}

type DeleteInput struct {
	ID string `path:"id"`
}

// currentUser builds a minimal *models.User from JWT claims.
// UserID and Role are embedded in the token — no DB lookup needed.
func currentUser(ctx context.Context) *models.User {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil
	}
	return &models.User{
		ID:   claims.UserID,
		Role: models.UserRole(claims.Role),
	}
}

func (h *Handler) List(ctx context.Context, _ *ListInput) (*ListOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	ids, err := h.integration.VisibleIDs(user.ID, user.IsAdmin())
	if err != nil {
		return nil, fmt.Errorf("fetch integrations: %w", err)
	}
	if len(ids) == 0 {
		return &ListOutput{Body: []mapper.IntegrationResponse{}}, nil
	}
	integrations, err := h.integration.ListVisible(ids)
	if err != nil {
		return nil, err
	}
	return &ListOutput{Body: mapper.ToIntegrationResponses(integrations, user.ID)}, nil
}

func (h *Handler) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	if input.Body.Provider == string(types.ProviderGiteaForgejo) && input.Body.BaseURL == "" {
		return nil, huma400("base_url is required for Gitea/Forgejo")
	}
	if input.Body.Provider == string(types.ProviderBitbucket) && input.Body.Username == "" {
		return nil, huma400("username is required for Bitbucket")
	}
	baseURL := input.Body.BaseURL
	if input.Body.Provider == string(types.ProviderBitbucket) {
		baseURL = input.Body.Username
	}
	encrypted, err := h.svc.PrepareSecret(input.Body.Token)
	if err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action:  logger.AuditGitTokenAdd,
				Success: false,
				Details: fmt.Sprintf("provider=%s name=%s encrypt_failed", input.Body.Provider, input.Body.Name),
			},
		)
		return nil, fmt.Errorf("secure token: %w", err)
	}
	m := &models.GitIntegration{
		UserID: user.ID, Name: input.Body.Name, Provider: input.Body.Provider,
		BaseURL: baseURL, AuthType: "token", SecretEncrypted: encrypted,
	}
	err = h.integration.Create(m)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditGitTokenAdd,
			ResourceID: m.ID,
			Success:    err == nil,
			Details:    fmt.Sprintf("provider=%s name=%s", input.Body.Provider, input.Body.Name),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("save integration: %w", err)
	}
	return &CreateOutput{Body: mapper.ToIntegrationResponse(m, user.ID)}, nil
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.LoadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	return &GetOutput{Body: mapper.ToIntegrationResponse(m, user.ID)}, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.RequireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	err = h.integration.Delete(input.ID)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditGitTokenDelete,
			ResourceID: input.ID,
			Success:    err == nil,
			Details:    fmt.Sprintf("provider=%s name=%s", m.Provider, m.Name),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("delete integration: %w", err)
	}
	return nil, nil
}

var _ = huma.Error403Forbidden
