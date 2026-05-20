package template

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Loader loads and caches templates from a directory.
type Loader struct {
	dir       string
	mu        sync.RWMutex
	templates map[string]*Template // keyed by slug
}

func NewLoader(dir, repoURL string) (*Loader, error) {
	if err := syncTemplates(dir, repoURL); err != nil {
		return nil, fmt.Errorf("sync templates: %w", err)
	}
	l := &Loader{
		dir:       dir,
		templates: make(map[string]*Template),
	}
	if err := l.Load(); err != nil {
		return nil, err
	}
	return l, nil
}

func syncTemplates(dir, repoURL string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	if repoURL == "" {
		return fmt.Errorf("TEMPLATES_DIR %q does not exist and TEMPLATES_REPO is not set", dir)
	}

	zipURL := strings.TrimSuffix(repoURL, ".git") + "/archive/refs/heads/main.zip"
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read zip: %w", err)
	}
	return unzipTemplates(body, dir)
}

func unzipTemplates(data []byte, dest string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		target := filepath.Join(dest, parts[1])
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// Load (re)reads all JSON and YAML template files from the directory.
func (l *Loader) Load() error {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return fmt.Errorf("read templates dir %q: %w", l.dir, err)
	}

	loaded := make(map[string]*Template)

	var loadEntry func(path string) error
	loadEntry = func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return nil
			}
			sub, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			for _, se := range sub {
				if err := loadEntry(filepath.Join(path, se.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !isTemplate(info.Name()) {
			return nil
		}
		t, err := loadFile(path)
		if err != nil {
			return err
		}
		loaded[t.Slug] = t
		return nil
	}

	for _, e := range entries {
		if err := loadEntry(filepath.Join(l.dir, e.Name())); err != nil {
			return err
		}
	}

	l.mu.Lock()
	l.templates = loaded
	l.mu.Unlock()
	return nil
}

// Get returns a template by slug.
func (l *Loader) Get(slug string) (*Template, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	t, ok := l.templates[slug]
	if !ok {
		return nil, fmt.Errorf("template %q not found", slug)
	}
	return t, nil
}

// List returns all loaded templates.
func (l *Loader) List() []*Template {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]*Template, 0, len(l.templates))
	for _, t := range l.templates {
		result = append(result, t)
	}
	return result
}

// ListByCategory returns templates filtered by category.
func (l *Loader) ListByCategory(category string) []*Template {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var result []*Template
	for _, t := range l.templates {
		if t.Category == category {
			result = append(result, t)
		}
	}
	return result
}

func loadFile(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template %q: %w", path, err)
	}

	var t Template
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("parse template %q: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("parse template %q: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported template format %q", ext)
	}

	if t.Slug == "" {
		return nil, fmt.Errorf("template %q missing slug", path)
	}
	return &t, nil
}

func isTemplate(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".json" || ext == ".yaml" || ext == ".yml"
}
