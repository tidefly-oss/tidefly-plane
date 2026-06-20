package git

import (
	"context"
	"fmt"
)

type listRepositoriesInput struct {
	ID string `path:"id"`
}

type listRepositoriesOutput struct {
	Body []Repository
}

type listBranchesInput struct {
	ID    string `path:"id"`
	Owner string `path:"owner"`
	Repo  string `path:"repo"`
}

type listBranchesOutput struct {
	Body []Branch
}

func (h *Handler) listRepositories(ctx context.Context, input *listRepositoriesInput) (*listRepositoriesOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.store.LoadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	repos, err := h.svc.ListRepositories(ctx, ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("fetch repositories: %w", err)
	}
	return &listRepositoriesOutput{Body: repos}, nil
}

func (h *Handler) listBranches(ctx context.Context, input *listBranchesInput) (*listBranchesOutput, error) {
	user := currentUser(ctx)
	if user == nil {
		return nil, huma401("unauthorized")
	}
	m, err := h.store.LoadVisible(input.ID, user.ID, user.IsAdmin())
	if err != nil {
		return nil, err
	}
	branches, err := h.svc.ListBranches(ctx, ProviderType(m.Provider), m.SecretEncrypted, m.BaseURL, input.Owner, input.Repo)
	if err != nil {
		return nil, fmt.Errorf("fetch branches: %w", err)
	}
	return &listBranchesOutput{Body: branches}, nil
}
