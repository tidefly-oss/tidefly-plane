package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/platform/version"
)

const (
	githubAPIBase   = "https://api.github.com/repos/tidefly-oss/%s/releases/latest"
	versionCacheTTL = 1 * time.Hour
)

// ── Types ─────────────────────────────────────────────────────────────────────

type ComponentVersion struct {
	Name            string `json:"name"`
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	Changelog       string `json:"changelog"`
	ReleaseURL      string `json:"release_url"`
	Prerelease      bool   `json:"prerelease"`
}

type versionInfo struct {
	Components         []ComponentVersion `json:"components"`
	AnyUpdateAvailable bool               `json:"any_update_available"`
}

// ── Cache ─────────────────────────────────────────────────────────────────────

type versionCache struct {
	mu        sync.RWMutex
	info      *versionInfo
	fetchedAt time.Time
}

var globalVersionCache = &versionCache{}

func (c *versionCache) get() *versionInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.info == nil || time.Since(c.fetchedAt) > versionCacheTTL {
		return nil
	}
	return c.info
}

func (c *versionCache) set(info *versionInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info = info
	c.fetchedAt = time.Now()
}

// ── GitHub fetcher ────────────────────────────────────────────────────────────

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Prerelease bool   `json:"prerelease"`
}

func fetchGitHubRelease(ctx context.Context, repo string) (*githubRelease, error) {
	url := fmt.Sprintf(githubAPIBase, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "tidefly-plane/"+version.Version)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d for %s", resp.StatusCode, repo)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// ── Current version detection ─────────────────────────────────────────────────

// getContainerVersion reads the image tag of a running container via Docker socket.
// Falls back to "unknown" if container is not running or label not set.
func (h *Handler) getContainerVersion(ctx context.Context, containerName string) string {
	details, err := h.runtime.GetContainer(ctx, containerName)
	if err != nil {
		return "unknown"
	}
	// Image tag is after the colon e.g. tidefly/tidefly-ui:v0.1.0-alpha.5
	parts := strings.SplitN(details.Image, ":", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return "unknown"
}

// ── Main fetch ────────────────────────────────────────────────────────────────

func (h *Handler) fetchVersionInfo(ctx context.Context) (*versionInfo, error) {
	if cached := globalVersionCache.get(); cached != nil {
		return cached, nil
	}

	type componentDef struct {
		name      string
		repo      string
		currentFn func() string
	}

	components := []componentDef{
		{
			name:      "plane",
			repo:      "tidefly-plane",
			currentFn: func() string { return currentVersion() },
		},
		{
			name: "ui",
			repo: "tidefly-ui",
			currentFn: func() string {
				return h.getContainerVersion(ctx, "tidefly_ui")
			},
		},
		{
			name: "agent",
			repo: "tidefly-agent",
			currentFn: func() string {
				return h.getContainerVersion(ctx, "tidefly_agent")
			},
		},
	}

	type result struct {
		idx  int
		comp ComponentVersion
	}

	results := make([]ComponentVersion, len(components))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, c := range components {
		wg.Add(1)
		go func(idx int, def componentDef) {
			defer wg.Done()
			current := def.currentFn()
			comp := ComponentVersion{
				Name:    def.name,
				Current: current,
				Latest:  "unknown",
			}

			release, err := fetchGitHubRelease(ctx, def.repo)
			if err == nil {
				comp.Latest = release.TagName
				comp.UpdateAvailable = isNewerVersion(release.TagName, current)
				comp.Changelog = release.Body
				comp.ReleaseURL = release.HTMLURL
				comp.Prerelease = release.Prerelease
			}

			mu.Lock()
			results[idx] = comp
			mu.Unlock()
		}(i, c)
	}

	wg.Wait()

	anyUpdate := false
	for _, c := range results {
		if c.UpdateAvailable {
			anyUpdate = true
			break
		}
	}

	info := &versionInfo{
		Components:         results,
		AnyUpdateAvailable: anyUpdate,
	}

	globalVersionCache.set(info)
	return info, nil
}

// ── Semver helpers ────────────────────────────────────────────────────────────

func isNewerVersion(latest, current string) bool {
	if current == "unknown" || current == "" {
		return false
	}
	l := strings.TrimPrefix(latest, "v")
	c := strings.TrimPrefix(current, "v")
	return l != c && semverGreater(l, c)
}

func semverGreater(a, b string) bool {
	aParts := strings.SplitN(a, "-", 2)
	bParts := strings.SplitN(b, "-", 2)

	aCore := strings.Split(aParts[0], ".")
	bCore := strings.Split(bParts[0], ".")

	for i := 0; i < 3; i++ {
		av := versionSegment(aCore, i)
		bv := versionSegment(bCore, i)
		if av != bv {
			return av > bv
		}
	}
	if len(aParts) == 1 && len(bParts) > 1 {
		return true
	}
	if len(aParts) > 1 && len(bParts) == 1 {
		return false
	}
	if len(aParts) > 1 && len(bParts) > 1 {
		return aParts[1] > bParts[1]
	}
	return false
}

func versionSegment(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(parts[i], "%d", &n)
	return n
}

func currentVersion() string {
	return version.Version
}
