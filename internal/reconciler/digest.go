package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// FetchDigest retrieves the OCI content digest for an image reference
// without pulling any layers — a single HEAD request to the registry.
// Supports Docker Hub, GHCR, and any OCI Distribution Spec registry.
// Auth via DefaultKeychain reads ~/.docker/config.json + env vars automatically.
func FetchDigest(ctx context.Context, image string) (string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return "", fmt.Errorf("parse image ref %q: %w", image, err)
	}
	desc, err := remote.Head(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	)
	if err != nil {
		return "", fmt.Errorf("registry HEAD %q: %w", image, err)
	}
	return desc.Digest.String(), nil
}

// PollDigests checks all running registry-backed services for new OCI digests.
// Called by the River UpdateCheckWorker every 6h.
// Drift (RemoteDigest != DeployedDigest) is picked up by the Reconciler
// on the next loop tick and triggers a blue-green or rolling update automatically.
func (r *Reconciler) PollDigests(ctx context.Context) {
	var services []models.Service
	if err := r.db.WithContext(ctx).
		Where("status = ? AND manifest_service = ?", models.ServiceStatusRunning, true).
		Find(&services).Error; err != nil {
		r.log.Error("reconciler", "poll digests: list services failed", err)
		return
	}

	now := time.Now().UTC()
	updated := 0

	for i := range services {
		svc := &services[i]
		image := resolveImage(*svc)
		if image == "" {
			continue // git/dockerfile source — no registry to poll
		}

		imgCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		digest, err := FetchDigest(imgCtx, image)
		cancel()

		if err != nil {
			// Silent skip — registry unreachable or rate limited, retried next cycle
			r.log.Info("reconciler", fmt.Sprintf("poll digests: %s fetch failed (skipping): %v", svc.Name, err))
			continue
		}

		fields := map[string]any{
			"remote_digest":     digest,
			"update_checked_at": now,
		}

		if digest != "" && digest != svc.DeployedDigest {
			fields["update_available"] = true
			fields["update_source"] = models.UpdateSourceRegistry
			updated++
			r.log.Info("reconciler", fmt.Sprintf("poll digests: %s new digest available %s", svc.Name, shortDigest(digest)))
		}

		r.db.WithContext(ctx).Model(svc).Updates(fields)
	}

	r.log.Info("reconciler", fmt.Sprintf("poll digests: checked %d services, %d updates available", len(services), updated))
}

// resolveImage extracts the registry image from a service's ManifestJSON.
// Returns empty string for git/compose/dockerfile-backed services (no registry to poll).
func resolveImage(svc models.Service) string {
	if svc.ManifestJSON == "" {
		return ""
	}
	var m struct {
		Spec struct {
			Container struct {
				Image string          `json:"image"`
				Build json.RawMessage `json:"build"`
			} `json:"container"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &m); err != nil {
		return ""
	}
	// Has a build spec → no registry image to poll
	if len(m.Spec.Container.Build) > 0 && string(m.Spec.Container.Build) != "null" {
		return ""
	}
	return m.Spec.Container.Image
}
