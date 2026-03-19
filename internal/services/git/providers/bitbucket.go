package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/services/git/types"
)

type bitbucketRepo struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
	MainBranch  struct {
		Name string `json:"name"`
	} `json:"mainbranch"`
	Links struct {
		Clone []struct {
			Name string `json:"name"`
			Href string `json:"href"`
		} `json:"clone"`
	} `json:"links"`
	UpdatedOn time.Time `json:"updated_on"`
}

type bitbucketBranch struct {
	Name   string `json:"name"`
	Target struct {
		Hash string `json:"hash"`
	} `json:"target"`
}

type bitbucketPage[T any] struct {
	Values []T    `json:"values"`
	Next   string `json:"next"`
}

type Bitbucket struct {
	username    string
	appPassword string
	client      *http.Client
}

func NewBitbucket(username, appPassword string) *Bitbucket {
	return &Bitbucket{username: username, appPassword: appPassword, client: &http.Client{Timeout: 15 * time.Second}}
}

func (b *Bitbucket) GetInfo() types.ProviderInfo {
	return types.ProviderInfo{Type: types.ProviderBitbucket, DisplayName: "Bitbucket", BaseURL: "https://bitbucket.org"}
}

func (b *Bitbucket) ValidateCredentials(ctx context.Context) error {
	req, err := b.newRequest(ctx, http.MethodGet, "https://api.bitbucket.org/2.0/user")
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("bitbucket: validating credentials: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("bitbucket: invalid credentials")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bitbucket: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (b *Bitbucket) ListRepositories(ctx context.Context) ([]types.Repository, error) {
	req, err := b.newRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s?pagelen=100&sort=-updated_on", b.username),
	)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: listing repositories: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket: unexpected status %d", resp.StatusCode)
	}
	var page bitbucketPage[bitbucketRepo]
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("bitbucket: decoding response: %w", err)
	}
	repos := make([]types.Repository, len(page.Values))
	for i, r := range page.Values {
		repos[i] = types.Repository{
			ID: r.UUID, Name: r.Name, FullName: r.FullName, Description: r.Description,
			CloneURL: b.extractCloneURL(r, "https"), SSHURL: b.extractCloneURL(r, "ssh"),
			Private: r.IsPrivate, DefaultBranch: r.MainBranch.Name, UpdatedAt: r.UpdatedOn,
		}
	}
	return repos, nil
}

func (b *Bitbucket) GetRepository(ctx context.Context, owner, name string) (*types.Repository, error) {
	req, err := b.newRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s", owner, name),
	)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: getting repository: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("bitbucket: repository %s/%s not found", owner, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket: unexpected status %d", resp.StatusCode)
	}
	var r bitbucketRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("bitbucket: decoding response: %w", err)
	}
	return &types.Repository{
		ID: r.UUID, Name: r.Name, FullName: r.FullName, Description: r.Description,
		CloneURL: b.extractCloneURL(r, "https"), SSHURL: b.extractCloneURL(r, "ssh"),
		Private: r.IsPrivate, DefaultBranch: r.MainBranch.Name, UpdatedAt: r.UpdatedOn,
	}, nil
}

func (b *Bitbucket) ListBranches(ctx context.Context, owner, name string) ([]types.Branch, error) {
	req, err := b.newRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/refs/branches?pagelen=100", owner, name),
	)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: listing branches: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket: unexpected status %d", resp.StatusCode)
	}
	var page bitbucketPage[bitbucketBranch]
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("bitbucket: decoding response: %w", err)
	}
	branches := make([]types.Branch, len(page.Values))
	for i, br := range page.Values {
		branches[i] = types.Branch{Name: br.Name, CommitSHA: br.Target.Hash}
	}
	return branches, nil
}

func (b *Bitbucket) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: creating request: %w", err)
	}
	req.SetBasicAuth(b.username, b.appPassword)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (b *Bitbucket) extractCloneURL(r bitbucketRepo, protocol string) string {
	for _, link := range r.Links.Clone {
		if link.Name == protocol {
			return link.Href
		}
	}
	return ""
}
