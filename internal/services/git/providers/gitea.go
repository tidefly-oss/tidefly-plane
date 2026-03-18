package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/services/git/types"
)

type giteaRepo struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	Private       bool      `json:"private"`
	DefaultBranch string    `json:"default_branch"`
	Updated       time.Time `json:"updated"`
}

type giteaBranch struct {
	Name   string `json:"name"`
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
}

type GiteaForgejo struct {
	token     string
	baseURL   string
	isForgejo bool
	client    *http.Client
}

func NewGitea(token, baseURL string) *GiteaForgejo {
	return &GiteaForgejo{token: token, baseURL: baseURL, client: &http.Client{Timeout: 15 * time.Second}}
}

func NewForgejo(token, baseURL string) *GiteaForgejo {
	return &GiteaForgejo{token: token, baseURL: baseURL, isForgejo: true, client: &http.Client{Timeout: 15 * time.Second}}
}

func (g *GiteaForgejo) GetInfo() types.ProviderInfo {
	displayName := "Gitea"
	if g.isForgejo {
		displayName = "Forgejo"
	}
	return types.ProviderInfo{Type: types.ProviderGiteaForgejo, DisplayName: displayName, BaseURL: g.baseURL}
}

func (g *GiteaForgejo) ValidateCredentials(ctx context.Context) error {
	req, err := g.newRequest(ctx, http.MethodGet, g.baseURL+"/api/v1/user")
	if err != nil {
		return err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("gitea: validating credentials: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("gitea: invalid or expired token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitea: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (g *GiteaForgejo) ListRepositories(ctx context.Context) ([]types.Repository, error) {
	req, err := g.newRequest(ctx, http.MethodGet, g.baseURL+"/api/v1/repos/search?limit=50&sort=newest")
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitea: listing repositories: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitea: unexpected status %d", resp.StatusCode)
	}
	var result struct {
		Data []giteaRepo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("gitea: decoding response: %w", err)
	}
	repos := make([]types.Repository, len(result.Data))
	for i, r := range result.Data {
		repos[i] = types.Repository{
			ID: fmt.Sprintf("%d", r.ID), Name: r.Name, FullName: r.FullName,
			Description: r.Description, CloneURL: r.CloneURL, SSHURL: r.SSHURL,
			Private: r.Private, DefaultBranch: r.DefaultBranch, UpdatedAt: r.Updated,
		}
	}
	return repos, nil
}

func (g *GiteaForgejo) GetRepository(ctx context.Context, owner, name string) (*types.Repository, error) {
	req, err := g.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/repos/%s/%s", g.baseURL, owner, name))
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitea: getting repository: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("gitea: repository %s/%s not found", owner, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitea: unexpected status %d", resp.StatusCode)
	}
	var r giteaRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("gitea: decoding response: %w", err)
	}
	return &types.Repository{
		ID: fmt.Sprintf("%d", r.ID), Name: r.Name, FullName: r.FullName,
		Description: r.Description, CloneURL: r.CloneURL, SSHURL: r.SSHURL,
		Private: r.Private, DefaultBranch: r.DefaultBranch, UpdatedAt: r.Updated,
	}, nil
}

func (g *GiteaForgejo) ListBranches(ctx context.Context, owner, name string) ([]types.Branch, error) {
	req, err := g.newRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/api/v1/repos/%s/%s/branches?limit=50", g.baseURL, owner, name),
	)
	if err != nil {
		return nil, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitea: listing branches: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitea: unexpected status %d", resp.StatusCode)
	}
	var raw []giteaBranch
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("gitea: decoding response: %w", err)
	}
	branches := make([]types.Branch, len(raw))
	for i, b := range raw {
		branches[i] = types.Branch{Name: b.Name, CommitSHA: b.Commit.ID}
	}
	return branches, nil
}

func (g *GiteaForgejo) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gitea: creating request: %w", err)
	}
	req.Header.Set("Authorization", "token "+g.token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
