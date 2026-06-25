package git

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
)

type listOutput struct {
	Body []integrationResponse
}

type gitCreateInput struct {
	Body struct {
		Name     string `json:"name"               minLength:"1" maxLength:"255"`
		Provider string `json:"provider"           enum:"github,gitlab,gitea-forgejo,bitbucket"`
		BaseURL  string `json:"base_url,omitempty"`
		Token    string `json:"token"              minLength:"1"`
		Username string `json:"username,omitempty" maxLength:"255"`
	}
}

type createOutput struct {
	Body integrationResponse
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body integrationResponse
}

type deleteInput struct {
	ID string `path:"id"`
}

func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	ids, err := h.store.VisibleIDs(user.ID, user.IsAdmin())
	if err != nil {
		return nil, fmt.Errorf("fetch integrations: %w", err)
	}
	if len(ids) == 0 {
		return &listOutput{Body: []integrationResponse{}}, nil
	}
	integrations, err := h.store.ListVisible(ids)
	if err != nil {
		return nil, err
	}
	return &listOutput{Body: toIntegrationResponses(integrations, user.ID)}, nil
}

func (h *Handler) create(ctx context.Context, input *gitCreateInput) (*createOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	if input.Body.Provider == string(ProviderGiteaForgejo) && input.Body.BaseURL == "" {
		return nil, huma400("base_url is required for Gitea/Forgejo")
	}
	if input.Body.Provider == string(ProviderBitbucket) && input.Body.Username == "" {
		return nil, huma400("username is required for Bitbucket")
	}
	baseURL := input.Body.BaseURL
	if input.Body.Provider == string(ProviderBitbucket) {
		baseURL = input.Body.Username
	}
	encrypted, err := h.svc.PrepareSecret(input.Body.Token)
	if err != nil {
		h.log.Audit(ctx, _logger.AuditEntry{
			Action:  _logger.AuditGitTokenAdd,
			Success: false,
			Details: fmt.Sprintf("provider=%s name=%s encrypt_failed", input.Body.Provider, input.Body.Name),
		})
		return nil, fmt.Errorf("secure token: %w", err)
	}
	m := &models.GitIntegration{
		UserID: user.ID, Name: input.Body.Name, Provider: input.Body.Provider,
		BaseURL: baseURL, AuthType: "token", SecretEncrypted: encrypted,
	}
	err = h.store.Create(m)
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditGitTokenAdd,
		ResourceID: m.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("provider=%s name=%s", input.Body.Provider, input.Body.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("save integration: %w", err)
	}
	h.bus.Publish(_eventbus.Event{
		Type:  _eventbus.EventGitIntegrationCreated,
		Topic: _eventbus.TopicGit,
		Payload: _eventbus.GitIntegrationPayload{
			ID:   m.ID,
			Name: m.Name,
		},
	})
	return &createOutput{Body: toIntegrationResponse(m, user.ID)}, nil
}

func (h *Handler) get(ctx context.Context, input *getInput) (*getOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.store.LoadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	return &getOutput{Body: toIntegrationResponse(m, user.ID)}, nil
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.store.RequireOwner(input.ID, user.ID)
	if err != nil {
		return nil, err
	}
	err = h.store.Delete(input.ID)
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditGitTokenDelete,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("provider=%s name=%s", m.Provider, m.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("delete integration: %w", err)
	}
	h.bus.Publish(_eventbus.Event{
		Type:  _eventbus.EventGitIntegrationDeleted,
		Topic: _eventbus.TopicGit,
		Payload: _eventbus.GitIntegrationPayload{
			ID:   input.ID,
			Name: m.Name,
		},
	})
	return nil, nil
}
