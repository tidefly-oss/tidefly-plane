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

	componentPlane = "plane"
	componentUI    = "ui"
	componentAgent = "agent"
	versionUnknown = "unknown"
)

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
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d for %s", resp.StatusCode, repo)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	release.Body = cleanChangelog(release.Body)
	return &release, nil
}

func cleanChangelog(body string) string {
	for _, marker := range []string{"## Docker", "## Platforms", "## Checksums", "## Installation"} {
		if idx := strings.Index(body, marker); idx != -1 {
			body = strings.TrimSpace(body[:idx])
		}
	}
	return body
}

func (h *Handler) getContainerVersion(ctx context.Context, containerName string) string {
	details, err := h.runtime.GetContainer(ctx, containerName)
	if err != nil {
		return versionUnknown
	}
	// Prefer label over image tag
	if v, ok := details.Labels["org.opencontainers.image.version"]; ok && v != "" {
		return v
	}
	parts := strings.SplitN(details.Image, ":", 2)
	if len(parts) == 2 && parts[1] != "latest" {
		return parts[1]
	}
	return versionUnknown
}

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
		{name: componentPlane, repo: "tidefly-plane", currentFn: currentVersion},
		{name: componentUI, repo: "tidefly-ui", currentFn: func() string {
			return h.getContainerVersion(ctx, "tidefly_ui")
		}},
		{name: componentAgent, repo: "tidefly-agent", currentFn: func() string {
			return h.getContainerVersion(ctx, "tidefly_agent")
		}},
	}
	results := make([]ComponentVersion, len(components))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, c := range components {
		wg.Add(1)
		go func(idx int, def componentDef) {
			defer wg.Done()
			current := def.currentFn()
			comp := ComponentVersion{Name: def.name, Current: current, Latest: versionUnknown}
			if release, err := fetchGitHubRelease(ctx, def.repo); err == nil {
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
	info := &versionInfo{Components: results, AnyUpdateAvailable: anyUpdate}
	globalVersionCache.set(info)
	return info, nil
}

func isNewerVersion(latest, current string) bool {
	if current == versionUnknown || current == "" {
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

func currentVersion() string { return version.Version }

// ── Version HTTP handler ──────────────────────────────────────────────────────

type VersionInput struct{}
type VersionOutput struct {
	Body versionInfo
}

func (h *Handler) Version(ctx context.Context, _ *VersionInput) (*VersionOutput, error) {
	info, err := h.fetchVersionInfo(ctx)
	if err != nil {
		h.log.Warnw("version_check", "failed to fetch version info", "error", err.Error())
		return &VersionOutput{Body: versionInfo{
			Components: []ComponentVersion{{
				Name:    componentPlane,
				Current: currentVersion(),
				Latest:  versionUnknown,
			}},
			AnyUpdateAvailable: false,
		}}, nil
	}
	return &VersionOutput{Body: *info}, nil
}
