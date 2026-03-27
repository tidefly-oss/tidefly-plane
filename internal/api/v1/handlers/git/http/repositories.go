package http

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/services/git/types"
)

type ListRepositoriesInput struct {
	ID string `path:"id"`
}
type ListRepositoriesOutput struct {
	Body []types.Repository
}

type ListBranchesInput struct {
	ID    string `path:"id"`
	Owner string `path:"owner"`
	Repo  string `path:"repo"`
}
type ListBranchesOutput struct {
	Body []types.Branch
}

func (h *Handler) ListRepositories(ctx context.Context, input *ListRepositoriesInput) (*ListRepositoriesOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.LoadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	repos, err := h.svc.ListRepositories(ctx, types.ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("fetch repositories: %w", err)
	}
	return &ListRepositoriesOutput{Body: repos}, nil
}

func (h *Handler) ListBranches(ctx context.Context, input *ListBranchesInput) (*ListBranchesOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.integration.LoadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	branches, err := h.svc.ListBranches(
		ctx, types.ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL, input.Owner, input.Repo,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch branches: %w", err)
	}
	return &ListBranchesOutput{Body: branches}, nil
}
