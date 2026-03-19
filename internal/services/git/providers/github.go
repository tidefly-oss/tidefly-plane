package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/services/git/types"
)

type githubRepo struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Description   string    `json:"description"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	Private       bool      `json:"private"`
	DefaultBranch string    `json:"default_branch"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type githubBranch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
	Protected bool `json:"protected"`
}

// GitHub implements the git.Provider interface for GitHub.com
type GitHub struct {
	token  string
	client *http.Client
}

// NewGitHub creates a new GitHub provider with the given personal access token.
func NewGitHub(token string) *GitHub {
	return &GitHub{
		token:  token,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *GitHub) GetInfo() types.ProviderInfo {
	return types.ProviderInfo{
		Type:        types.ProviderGitHub,
		DisplayName: "GitHub",
		BaseURL:     "https://github.com",
	}
}

func (g *GitHub) ValidateCredentials(ctx context.Context) error {
	req, err := g.newRequest(ctx, http.MethodGet, "https://api.github.com/user")
	if err != nil {
		return err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("github: validating credentials: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("github: invalid or expired token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (g *GitHub) ListRepositories(ctx context.Context) ([]types.Repository, error) {
	url := "https://api.github.com/user/repos?per_page=100&sort=updated&affiliation=owner,collaborator,organization_member"
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: listing repositories: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: unexpected status %d", resp.StatusCode)
	}

	var raw []githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("github: decoding response: %w", err)
	}

	repos := make([]types.Repository, len(raw))
	for i, r := range raw {
		repos[i] = types.Repository{
			ID:            fmt.Sprintf("%d", r.ID),
			Name:          r.Name,
			FullName:      r.FullName,
			Description:   r.Description,
			CloneURL:      r.CloneURL,
			SSHURL:        r.SSHURL,
			Private:       r.Private,
			DefaultBranch: r.DefaultBranch,
			UpdatedAt:     r.UpdatedAt,
		}
	}
	return repos, nil
}

func (g *GitHub) GetRepository(ctx context.Context, owner, name string) (*types.Repository, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, name)
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: getting repository: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("github: repository %s/%s not found", owner, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: unexpected status %d", resp.StatusCode)
	}

	var r githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("github: decoding response: %w", err)
	}

	repo := &types.Repository{
		ID:            fmt.Sprintf("%d", r.ID),
		Name:          r.Name,
		FullName:      r.FullName,
		Description:   r.Description,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		Private:       r.Private,
		DefaultBranch: r.DefaultBranch,
		UpdatedAt:     r.UpdatedAt,
	}
	return repo, nil
}

func (g *GitHub) ListBranches(ctx context.Context, owner, name string) ([]types.Branch, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches?per_page=100", owner, name)
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: listing branches: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: unexpected status %d", resp.StatusCode)
	}

	var raw []githubBranch
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("github: decoding response: %w", err)
	}

	branches := make([]types.Branch, len(raw))
	for i, b := range raw {
		branches[i] = types.Branch{
			Name:      b.Name,
			CommitSHA: b.Commit.SHA,
			Protected: b.Protected,
		}
	}
	return branches, nil
}

func (g *GitHub) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("github: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return req, nil
}
