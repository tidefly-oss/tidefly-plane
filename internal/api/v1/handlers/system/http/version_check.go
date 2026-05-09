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
	githubReleasesURL = "https://api.github.com/repos/tidefly-oss/tidefly-plane/releases/latest"
	versionCacheTTL   = 1 * time.Hour
)

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

type versionInfo struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	Changelog       string `json:"changelog"`
	ReleaseURL      string `json:"release_url"`
	Prerelease      bool   `json:"prerelease"`
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

func fetchLatestVersion(ctx context.Context) (*versionInfo, error) {
	if cached := globalVersionCache.get(); cached != nil {
		return cached, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "tidefly-plane/"+version.Version)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	info := &versionInfo{
		Current:         version.Version,
		Latest:          release.TagName,
		UpdateAvailable: isNewerVersion(release.TagName, version.Version),
		Changelog:       release.Body,
		ReleaseURL:      release.HTMLURL,
		Prerelease:      release.Prerelease,
	}

	globalVersionCache.set(info)
	return info, nil
}

// isNewerVersion compares semver strings — strips leading "v", compares lexicographically.
// Good enough for tidefly's versioning scheme (v0.1.0-alpha.29 etc).
func isNewerVersion(latest, current string) bool {
	l := strings.TrimPrefix(latest, "v")
	c := strings.TrimPrefix(current, "v")
	return l != c && semverGreater(l, c)
}

func semverGreater(a, b string) bool {
	// Split on "-" to separate pre-release
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
	// Same core version — no pre-release > pre-release
	if len(aParts) == 1 && len(bParts) > 1 {
		return true
	}
	if len(aParts) > 1 && len(bParts) == 1 {
		return false
	}
	// Both have pre-release — compare lexicographically
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
