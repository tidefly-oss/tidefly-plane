package template

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultRepoOwner = "tidefly-oss"
	defaultRepoName  = "tidefly-templates"
	defaultBranch    = "main"

	indexCacheTTL    = 5 * time.Minute
	templateCacheTTL = 30 * time.Minute
	fetchTimeout     = 15 * time.Second
)

type Loader struct {
	owner  string
	repo   string
	branch string
	client *http.Client

	indexMu      sync.RWMutex
	index        []Summary
	indexFetched time.Time

	tmplMu    sync.RWMutex
	templates map[string]*cachedTemplate
}

type cachedTemplate struct {
	tmpl    *Template
	fetched time.Time
}

func NewLoader() *Loader {
	return &Loader{
		owner:     defaultRepoOwner,
		repo:      defaultRepoName,
		branch:    defaultBranch,
		client:    &http.Client{Timeout: fetchTimeout},
		templates: make(map[string]*cachedTemplate),
	}
}

func (l *Loader) rawURL(path string) string {
	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/%s",
		l.owner, l.repo, l.branch, path,
	)
}

// ── Index ─────────────────────────────────────────────────────────────────────

func (l *Loader) List() ([]Summary, error) {
	if err := l.refreshIndexIfStale(); err != nil {
		return nil, err
	}
	l.indexMu.RLock()
	defer l.indexMu.RUnlock()
	return l.index, nil
}

func (l *Loader) ListByCategory(category string) ([]Summary, error) {
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	var result []Summary
	for _, s := range all {
		if s.Category == category {
			result = append(result, s)
		}
	}
	return result, nil
}

func (l *Loader) ListByTag(tag string) ([]Summary, error) {
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	var result []Summary
	for _, s := range all {
		for _, t := range s.Tags {
			if t == tag {
				result = append(result, s)
				break
			}
		}
	}
	return result, nil
}

func (l *Loader) refreshIndexIfStale() error {
	l.indexMu.RLock()
	stale := time.Since(l.indexFetched) > indexCacheTTL
	l.indexMu.RUnlock()
	if !stale {
		return nil
	}
	return l.fetchIndex()
}

func (l *Loader) fetchIndex() error {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.rawURL("index.json"), nil)
	if err != nil {
		return err
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch index.json: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch index.json: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read index.json: %w", err)
	}

	var summaries []Summary
	if err := json.Unmarshal(data, &summaries); err != nil {
		return fmt.Errorf("parse index.json: %w", err)
	}

	l.indexMu.Lock()
	l.index = summaries
	l.indexFetched = time.Now()
	l.indexMu.Unlock()

	return nil
}

// ── Individual templates ───────────────────────────────────────────────────────

func (l *Loader) Get(slug string) (*Template, error) {
	l.tmplMu.RLock()
	cached, ok := l.templates[slug]
	l.tmplMu.RUnlock()

	if ok && time.Since(cached.fetched) < templateCacheTTL {
		return cached.tmpl, nil
	}

	return l.fetchTemplate(slug)
}

func (l *Loader) fetchTemplate(slug string) (*Template, error) {
	// Find the path from the index — category/slug.json
	summary, err := l.findSummary(slug)
	if err != nil {
		return nil, fmt.Errorf("template %q not found in index", slug)
	}

	path := summary.Category + "/" + slug + ".json"
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.rawURL(path), nil)
	if err != nil {
		return nil, err
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch template %q: %w", slug, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("template %q not found", slug)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch template %q: HTTP %d", slug, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read template %q: %w", slug, err)
	}

	var t Template
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse template %q: %w", slug, err)
	}
	if t.Slug == "" {
		t.Slug = slug
	}

	l.tmplMu.Lock()
	l.templates[slug] = &cachedTemplate{tmpl: &t, fetched: time.Now()}
	l.tmplMu.Unlock()

	return &t, nil
}

func (l *Loader) findSummary(slug string) (*Summary, error) {
	if err := l.refreshIndexIfStale(); err != nil {
		return nil, err
	}
	l.indexMu.RLock()
	defer l.indexMu.RUnlock()
	for i := range l.index {
		if l.index[i].Slug == slug {
			return &l.index[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (l *Loader) InvalidateTemplate(slug string) {
	l.tmplMu.Lock()
	delete(l.templates, slug)
	l.tmplMu.Unlock()
}

func (l *Loader) InvalidateAll() {
	l.indexMu.Lock()
	l.index = nil
	l.indexFetched = time.Time{}
	l.indexMu.Unlock()

	l.tmplMu.Lock()
	l.templates = make(map[string]*cachedTemplate)
	l.tmplMu.Unlock()
}
