package template

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultRepoOwner = "tidefly-oss"
	defaultRepoName  = "tidefly-templates"
	defaultBranch    = "main"
	cacheTTL         = 5 * time.Minute
	fetchTimeout     = 15 * time.Second
)

// Loader fetches and caches templates from GitHub.
// No filesystem dependency — templates are always loaded from the repo.
type Loader struct {
	owner  string
	repo   string
	branch string
	client *http.Client

	mu          sync.RWMutex
	templates   map[string]*Template
	lastFetched time.Time
}

func NewLoader() *Loader {
	return &Loader{
		owner:     defaultRepoOwner,
		repo:      defaultRepoName,
		branch:    defaultBranch,
		client:    &http.Client{Timeout: fetchTimeout},
		templates: make(map[string]*Template),
	}
}

// List returns all templates, refreshing from GitHub if the cache is stale.
func (l *Loader) List() ([]*Template, error) {
	if err := l.refreshIfStale(); err != nil {
		return nil, err
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]*Template, 0, len(l.templates))
	for _, t := range l.templates {
		result = append(result, t)
	}
	return result, nil
}

// Get returns a single template by slug, refreshing if stale.
func (l *Loader) Get(slug string) (*Template, error) {
	if err := l.refreshIfStale(); err != nil {
		return nil, err
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	t, ok := l.templates[slug]
	if !ok {
		return nil, fmt.Errorf("template %q not found", slug)
	}
	return t, nil
}

// ListByCategory returns templates filtered by category.
func (l *Loader) ListByCategory(category string) ([]*Template, error) {
	templates, err := l.List()
	if err != nil {
		return nil, err
	}
	var result []*Template
	for _, t := range templates {
		if t.Category == category {
			result = append(result, t)
		}
	}
	return result, nil
}

// refreshIfStale fetches templates from GitHub if the cache has expired.
func (l *Loader) refreshIfStale() error {
	l.mu.RLock()
	stale := time.Since(l.lastFetched) > cacheTTL
	l.mu.RUnlock()

	if !stale {
		return nil
	}

	return l.fetch()
}

// fetch downloads the full template list from GitHub and updates the cache.
func (l *Loader) fetch() error {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	// GitHub API: get all files in the repo tree
	treeURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1",
		l.owner, l.repo, l.branch,
	)
	tree, err := l.fetchTree(ctx, treeURL)
	if err != nil {
		return fmt.Errorf("fetch template tree: %w", err)
	}

	loaded := make(map[string]*Template)
	for _, item := range tree.Tree {
		if item.Type != "blob" || !isTemplate(item.Path) {
			continue
		}
		rawURL := fmt.Sprintf(
			"https://raw.githubusercontent.com/%s/%s/%s/%s",
			l.owner, l.repo, l.branch, item.Path,
		)
		t, err := l.fetchTemplate(ctx, rawURL, item.Path)
		if err != nil {
			// Log and skip malformed templates — don't abort the whole list
			// TODO: surface via structured logger once injected
			_ = fmt.Errorf("skip template %q: %w", item.Path, err)
			continue
		}
		loaded[t.Slug] = t
	}

	l.mu.Lock()
	l.templates = loaded
	l.lastFetched = time.Now()
	l.mu.Unlock()

	return nil
}

type githubTree struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

func (l *Loader) fetchTree(ctx context.Context, url string) (*githubTree, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var tree githubTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, fmt.Errorf("decode tree: %w", err)
	}
	return &tree, nil
}

func isTemplate(path string) bool {
	return len(path) > 5 && path[len(path)-5:] == ".json"
}

func (l *Loader) fetchTemplate(ctx context.Context, rawURL string, path string) (*Template, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var t Template
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if t.Slug == "" {
		return nil, fmt.Errorf("missing slug")
	}
	// Derive category from folder name if not set in JSON
	if t.Category == "" {
		if parts := strings.SplitN(path, "/", 2); len(parts) == 2 {
			t.Category = parts[0]
		}
	}
	return &t, nil
}
