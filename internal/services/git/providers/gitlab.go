package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/services/git/types"
)

const defaultGitLabBaseURL = "https://gitlab.com"

type gitlabProject struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	PathWithNamespace string    `json:"path_with_namespace"`
	Description       string    `json:"description"`
	HTTPURLToRepo     string    `json:"http_url_to_repo"`
	SSHURLToRepo      string    `json:"ssh_url_to_repo"`
	Visibility        string    `json:"visibility"`
	DefaultBranch     string    `json:"default_branch"`
	LastActivityAt    time.Time `json:"last_activity_at"`
}

type gitlabBranch struct {
	Name   string `json:"name"`
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
	Protected bool `json:"protected"`
}

// GitLab implements git.Provider for GitLab.com and self-hosted GitLab instances.
type GitLab struct {
	token   string
	baseURL string
	client  *http.Client
}

// NewGitLab creates a new GitLab provider.
// Pass baseURL as empty string to use gitlab.com, or a custom URL for self-hosted.
func NewGitLab(token, baseURL string) *GitLab {
	if baseURL == "" {
		baseURL = defaultGitLabBaseURL
	}
	return &GitLab{
		token:   token,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *GitLab) GetInfo() types.ProviderInfo {
	return types.ProviderInfo{
		Type:        types.ProviderGitLab,
		DisplayName: "GitLab",
		BaseURL:     g.baseURL,
	}
}

func (g *GitLab) ValidateCredentials(ctx context.Context) error {
	req, err := g.newRequest(ctx, http.MethodGet, g.baseURL+"/api/v4/user")
	if err != nil {
		return err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab: validating credentials: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("gitlab: invalid or expired token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitlab: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (g *GitLab) ListRepositories(ctx context.Context) ([]types.Repository, error) {
	url := g.baseURL + "/api/v4/projects?membership=true&per_page=100&order_by=last_activity_at"
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: listing repositories: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab: unexpected status %d", resp.StatusCode)
	}

	var raw []gitlabProject
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("gitlab: decoding response: %w", err)
	}

	repos := make([]types.Repository, len(raw))
	for i, p := range raw {
		repos[i] = types.Repository{
			ID:            fmt.Sprintf("%d", p.ID),
			Name:          p.Name,
			FullName:      p.PathWithNamespace,
			Description:   p.Description,
			CloneURL:      p.HTTPURLToRepo,
			SSHURL:        p.SSHURLToRepo,
			Private:       p.Visibility == "private",
			DefaultBranch: p.DefaultBranch,
			UpdatedAt:     p.LastActivityAt,
		}
	}
	return repos, nil
}

func (g *GitLab) GetRepository(ctx context.Context, owner, name string) (*types.Repository, error) {
	encoded := owner + "%2F" + name
	url := fmt.Sprintf("%s/api/v4/projects/%s", g.baseURL, encoded)
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: getting repository: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("gitlab: repository %s/%s not found", owner, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab: unexpected status %d", resp.StatusCode)
	}

	var p gitlabProject
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("gitlab: decoding response: %w", err)
	}

	return &types.Repository{
		ID:            fmt.Sprintf("%d", p.ID),
		Name:          p.Name,
		FullName:      p.PathWithNamespace,
		Description:   p.Description,
		CloneURL:      p.HTTPURLToRepo,
		SSHURL:        p.SSHURLToRepo,
		Private:       p.Visibility == "private",
		DefaultBranch: p.DefaultBranch,
		UpdatedAt:     p.LastActivityAt,
	}, nil
}

func (g *GitLab) ListBranches(ctx context.Context, owner, name string) ([]types.Branch, error) {
	encoded := owner + "%2F" + name
	url := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches?per_page=100", g.baseURL, encoded)
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: listing branches: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gitlab: unexpected status %d", resp.StatusCode)
	}

	var raw []gitlabBranch
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("gitlab: decoding response: %w", err)
	}

	branches := make([]types.Branch, len(raw))
	for i, b := range raw {
		branches[i] = types.Branch{
			Name:      b.Name,
			CommitSHA: b.Commit.ID,
			Protected: b.Protected,
		}
	}
	return branches, nil
}

func (g *GitLab) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gitlab: creating request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", g.token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
