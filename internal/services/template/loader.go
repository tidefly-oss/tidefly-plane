package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Loader loads and caches templates from a directory.
type Loader struct {
	dir       string
	mu        sync.RWMutex
	templates map[string]*Template // keyed by slug
}

func NewLoader(dir string) (*Loader, error) {
	l := &Loader{
		dir:       dir,
		templates: make(map[string]*Template),
	}
	if err := l.Load(); err != nil {
		return nil, err
	}
	return l, nil
}

// Load (re)reads all YAML files from the template directory.
func (l *Loader) Load() error {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return fmt.Errorf("read templates dir %q: %w", l.dir, err)
	}

	loaded := make(map[string]*Template)
	for _, e := range entries {
		if e.IsDir() {
			// Skip hidden directories (starting with .)
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			// recurse one level (e.g. databases/, messaging/)
			sub := filepath.Join(l.dir, e.Name())
			subEntries, err := os.ReadDir(sub)
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if !isYAML(se.Name()) {
					continue
				}
				t, err := loadFile(filepath.Join(sub, se.Name()))
				if err != nil {
					return err
				}
				loaded[t.Slug] = t
			}
		} else if isYAML(e.Name()) {
			t, err := loadFile(filepath.Join(l.dir, e.Name()))
			if err != nil {
				return err
			}
			loaded[t.Slug] = t
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
	result := make([]*Template, 0)
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
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse template %q: %w", path, err)
	}
	if t.Slug == "" {
		return nil, fmt.Errorf("template %q missing slug", path)
	}
	return &t, nil
}

func isYAML(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}
