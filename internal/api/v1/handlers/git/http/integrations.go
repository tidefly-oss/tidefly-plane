package http

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/git/mapper"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git/types"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type GitListInput struct{}
type GitListOutput struct {
	Body []mapper.IntegrationResponse
}

type GitCreateInput struct {
	Body struct {
		Name     string `json:"name"               minLength:"1" maxLength:"255"`
		Provider string `json:"provider"           enum:"github,gitlab,gitea-forgejo,bitbucket"`
		BaseURL  string `json:"base_url,omitempty"`
		Token    string `json:"token"              minLength:"1"`
		Username string `json:"username,omitempty" maxLength:"255"`
	}
}
type GitCreateOutput struct {
	Body mapper.IntegrationResponse
}

type GitGetInput struct {
	ID string `path:"id"`
}
type GitGetOutput struct {
	Body mapper.IntegrationResponse
}

type GitDeleteInput struct {
	ID string `path:"id"`
}

// currentUser builds a minimal *models.User from JWT claims.
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

func (h *Handler) List(ctx context.Context, _ *GitListInput) (*GitListOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	ids, err := h.integration.VisibleIDs(user.ID, user.IsAdmin())
	if err != nil {
		return nil, fmt.Errorf("fetch integrations: %w", err)
	}
	if len(ids) == 0 {
		return &GitListOutput{Body: []mapper.IntegrationResponse{}}, nil
	}
	integrations, err := h.integration.ListVisible(ids)
	if err != nil {
		return nil, err
	}
	return &GitListOutput{Body: mapper.ToIntegrationResponses(integrations, user.ID)}, nil
}

func (h *Handler) Create(ctx context.Context, input *GitCreateInput) (*GitCreateOutput, error) {
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
		h.log.Audit(ctx, logger.AuditEntry{
			Action:  logger.AuditGitTokenAdd,
			Success: false,
			Details: fmt.Sprintf("provider=%s name=%s encrypt_failed", input.Body.Provider, input.Body.Name),
		})
		return nil, fmt.Errorf("secure token: %w", err)
	}
	m := &models.GitIntegration{
		UserID: user.ID, Name: input.Body.Name, Provider: input.Body.Provider,
		BaseURL: baseURL, AuthType: "token", SecretEncrypted: encrypted,
	}
	err = h.integration.Create(m)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditGitTokenAdd,
		ResourceID: m.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("provider=%s name=%s", input.Body.Provider, input.Body.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("save integration: %w", err)
	}
	h.bus.Publish(eventbus.Event{
		Type:  eventbus.EventGitIntegrationCreated,
		Topic: eventbus.TopicGit,
		Payload: eventbus.GitIntegrationPayload{
			ID:   m.ID,
			Name: m.Name,
		},
	})
	return &GitCreateOutput{Body: mapper.ToIntegrationResponse(m, user.ID)}, nil
}

func (h *Handler) Get(ctx context.Context, input *GitGetInput) (*GitGetOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.LoadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	return &GitGetOutput{Body: mapper.ToIntegrationResponse(m, user.ID)}, nil
}

func (h *Handler) Delete(ctx context.Context, input *GitDeleteInput) (*struct{}, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.RequireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	err = h.integration.Delete(input.ID)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditGitTokenDelete,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("provider=%s name=%s", m.Provider, m.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("delete integration: %w", err)
	}
	h.bus.Publish(eventbus.Event{
		Type:  eventbus.EventGitIntegrationDeleted,
		Topic: eventbus.TopicGit,
		Payload: eventbus.GitIntegrationPayload{
			ID:   input.ID,
			Name: m.Name,
		},
	})
	return nil, nil
}

var _ = huma.Error403Forbidden
