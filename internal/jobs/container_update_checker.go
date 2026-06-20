package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
)

const TaskUpdateCheck = "services:update_check"

// updateCheckResult holds the outcome for a single service digest check.
type updateCheckResult struct {
	serviceID   string
	serviceName string
	digest      string
	hasUpdate   bool
	err         error
}

// HandleUpdateCheck is the asynq handler for the periodic digest check job.
// Runs every 6h via scheduler. Uses a semaphore (max 3 concurrent) to avoid
// hammering the registry or the Docker socket.
func (h *Handler) HandleUpdateCheck(ctx context.Context, _ *asynq.Task) error {
	h.log.Info("update_checker", "starting digest check for all registry-backed services")

	var services []models.Service
	if err := h.db.WithContext(ctx).
		Where("status = ?", models.ServiceStatusRunning).
		Find(&services).Error; err != nil {
		return fmt.Errorf("update_checker: list services: %w", err)
	}

	if len(services) == 0 {
		h.log.Info("update_checker", "no running services to check")
		return nil
	}

	sem := make(chan struct{}, 3) // max 3 concurrent registry calls
	results := make(chan updateCheckResult, len(services))
	var wg sync.WaitGroup

	for _, svc := range services {
		image := resolveServiceImage(svc)
		if image == "" {
			continue // git/compose source — no registry digest to check
		}

		wg.Add(1)
		go func(s models.Service, img string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			remoteDigest, err := fetchRemoteDigest(ctx, img)
			if err != nil {
				results <- updateCheckResult{serviceID: s.ID.String(), err: err}
				return
			}

			hasUpdate := remoteDigest != "" && remoteDigest != s.RemoteDigest
			results <- updateCheckResult{
				serviceID:   s.ID.String(),
				serviceName: s.Name,
				digest:      remoteDigest,
				hasUpdate:   hasUpdate,
			}
		}(svc, image)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	now := time.Now().UTC()
	updated := 0

	for r := range results {
		if r.err != nil {
			// Silent skip — registry unreachable or rate limited; retried next cycle.
			h.log.Info("update_checker", fmt.Sprintf("service %s: digest fetch failed (skipping): %v", r.serviceID, r.err))
			continue
		}

		fields := map[string]any{
			"remote_digest":     r.digest,
			"update_checked_at": now,
		}
		if r.hasUpdate {
			fields["update_available"] = true
			fields["update_source"] = models.UpdateSourceRegistry
		}

		if err := h.db.WithContext(ctx).
			Model(&models.Service{}).
			Where("id = ?", r.serviceID).
			Updates(fields).Error; err != nil {
			h.log.Info("update_checker", fmt.Sprintf("service %s: db update failed: %v", r.serviceID, err))
			continue
		}

		if r.hasUpdate {
			updated++
			// Publish via notifSvc — writes to DB + pushes SSE to UI in one call.
			if h.notifSvc != nil {
				_ = h.notifSvc.Publish(
					ctx,
					models.SeverityInfo,
					"Update verfügbar",
					fmt.Sprintf("Service \"%s\" hat ein neues Image verfügbar.", r.serviceName),
				)
			}
		}
	}

	h.log.Info("update_checker", fmt.Sprintf("digest check complete — %d/%d services have updates", updated, len(services)))
	return nil
}

// MarkUpdateAvailableByTemplateSlug is called by the webhook handler when
// tidefly-templates pushes an update_notify event. Sets update_available on
// all running services that use the given template slug.
func MarkUpdateAvailableByTemplateSlug(ctx context.Context, db *gorm.DB, slug string) (int64, error) {
	result := db.WithContext(ctx).
		Model(&models.Service{}).
		Where("template_slug = ? AND status = ?", slug, models.ServiceStatusRunning).
		Updates(map[string]any{
			"update_available": true,
			"update_source":    models.UpdateSourceTemplate,
		})
	return result.RowsAffected, result.Error
}

// MarkUpdateAvailableByServiceID is called by the webhook handler when a
// user's GitHub repo fires a push event linked to a service.
func MarkUpdateAvailableByServiceID(ctx context.Context, db *gorm.DB, serviceID string) error {
	return db.WithContext(ctx).
		Model(&models.Service{}).
		Where("id = ?", serviceID).
		Updates(map[string]any{
			"update_available": true,
			"update_source":    models.UpdateSourceGit,
		}).Error
}

// ResetUpdateAvailable clears the update flag after a successful deploy.
// Call this at the end of HandleServiceUpdate / HandleServiceRedeploy.
func ResetUpdateAvailable(ctx context.Context, db *gorm.DB, serviceID string) error {
	return db.WithContext(ctx).
		Model(&models.Service{}).
		Where("id = ?", serviceID).
		Updates(map[string]any{
			"update_available": false,
			"update_source":    "",
		}).Error
}

// resolveServiceImage extracts the registry image reference from a service's ManifestJSON.
// Returns empty string if the service is git/compose-backed with no image field.
func resolveServiceImage(svc models.Service) string {
	if svc.ManifestJSON == "" {
		return ""
	}
	var manifest struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &manifest); err != nil {
		return ""
	}
	return manifest.Image
}

// fetchRemoteDigest retrieves the content digest of a registry image
// without pulling any layers — a single HTTPS call to the registry manifest API.
//
// Supports Docker Hub (with/without explicit registry prefix) and any registry
// that implements the OCI Distribution Spec.
func fetchRemoteDigest(ctx context.Context, image string) (string, error) {
	registry, repo, tag := parseImageRef(image)

	token, err := fetchDockerHubToken(ctx, registry, repo)
	if err != nil {
		return "", fmt.Errorf("auth token: %w", err)
	}

	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned %d for %s", resp.StatusCode, image)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no Docker-Content-Digest header for %s", image)
	}
	return digest, nil
}

// fetchDockerHubToken obtains an anonymous Bearer token for Docker Hub.
// Returns empty string for non-Docker Hub registries (public images need no auth).
func fetchDockerHubToken(ctx context.Context, registry, repo string) (string, error) {
	if registry != "registry-1.docker.io" {
		return "", nil
	}
	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	return result.Token, nil
}

// parseImageRef splits an image reference into (registry, repository, tag).
//
//	"nginx:latest"                    → registry-1.docker.io, library/nginx, latest
//	"myuser/myapp:v1.2"               → registry-1.docker.io, myuser/myapp, v1.2
//	"ghcr.io/tidefly-oss/plane:main"  → ghcr.io, tidefly-oss/plane, main
func parseImageRef(image string) (registry, repo, tag string) {
	tag = "latest"
	if i := strings.LastIndex(image, ":"); i != -1 && !strings.Contains(image[i:], "/") {
		tag = image[i+1:]
		image = image[:i]
	}
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 && strings.ContainsAny(parts[0], ".:") {
		registry = parts[0]
		repo = parts[1]
	} else {
		registry = "registry-1.docker.io"
		if len(parts) == 1 {
			repo = "library/" + parts[0]
		} else {
			repo = image
		}
	}
	return
}
