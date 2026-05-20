package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/converter"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/manifest"
	caddyingress "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

func (h *ServiceJobHandler) HandleServiceDeploy(ctx context.Context, t *asynq.Task) error {
	var p ServiceDeployPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal deploy payload: %w", err)
	}

	h.log.Info("jobs", fmt.Sprintf("service deploy started: id=%s", p.ServiceID))

	var svc models.Service
	if err := h.db.First(&svc, "id = ?", p.ServiceID).Error; err != nil {
		return fmt.Errorf("service %q not found: %w", p.ServiceID, err)
	}

	// ── 1. Convert ────────────────────────────────────────────────────────────
	conv := converter.New()
	result, err := conv.Convert(ctx, p.Input.ToConvertInput(p.GitToken))
	if err != nil {
		h.markFailed(&svc, err)
		return fmt.Errorf("convert: %w", err)
	}
	raw := result.Manifests[0]

	// ── 2. Resolve ────────────────────────────────────────────────────────────
	resolved, err := manifest.Resolve(raw)
	if err != nil {
		h.markFailed(&svc, err)
		return fmt.Errorf("resolve manifest: %w", err)
	}

	if resolved.Name != "" && svc.Name != resolved.Name {
		h.db.Model(&svc).Update("name", resolved.Name)
		svc.Name = resolved.Name
	}

	// ── 3. Networks ───────────────────────────────────────────────────────────
	projectNetwork, err := h.resolveProjectNetwork(svc.ProjectID)
	if err != nil {
		h.markFailed(&svc, err)
		return err
	}

	if err := h.ensureNetwork(ctx, proxyNetwork); err != nil {
		h.markFailed(&svc, err)
		return err
	}

	primaryNetwork := proxyNetwork
	if projectNetwork != "" {
		if err := h.ensureNetwork(ctx, projectNetwork); err != nil {
			h.markFailed(&svc, err)
			return err
		}
		primaryNetwork = projectNetwork
	}

	// ── 4. Build ──────────────────────────────────────────────────────────────
	if result.BuildRequired {
		h.log.Info("jobs", fmt.Sprintf("building image %q for service %q", result.BuildTag, svc.Name))
		if err := h.buildImage(ctx, result); err != nil {
			h.markFailed(&svc, err)
			return fmt.Errorf("build image: %w", err)
		}
	}

	// ── 5. Volumes ────────────────────────────────────────────────────────────
	for _, v := range resolved.Container.Volumes {
		if err := h.rt.CreateVolume(ctx, v.Name); err != nil {
			h.markFailed(&svc, err)
			return fmt.Errorf("create volume %q: %w", v.Name, err)
		}
	}

	// ── 6. Container ──────────────────────────────────────────────────────────
	isPodman := h.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(resolved, primaryNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.service"] = svc.Name
	spec.Labels["tidefly.project"] = svc.ProjectID

	if err := h.rt.CreateContainer(ctx, spec); err != nil {
		h.markFailed(&svc, err)
		return fmt.Errorf("create container: %w", err)
	}

	// ── 7. Connect to proxy if exposed and in project network ─────────────────
	if raw.Spec.Expose != nil && primaryNetwork != proxyNetwork {
		if err := h.rt.ConnectNetwork(ctx, svc.Name, proxyNetwork); err != nil {
			h.log.Warnw("jobs", "failed to connect container to proxy network", "service", svc.Name, "error", err)
		}
	}

	// ── 8. Caddy route ────────────────────────────────────────────────────────
	var publicURL string
	if resolved.Expose.Domain != "" {
		route := caddyingress.RouteFromManifest(resolved.Name, resolved.Expose.Domain, resolved.Expose.Port, resolved.Expose.TLS, resolved.Expose.WWW)
		if routeErr := h.ingress.AddRoute(ctx, route); routeErr != nil {
			h.log.Warnw("jobs", "failed to register ingress route", "service", resolved.Name, "error", routeErr)
		} else {
			scheme := "https"
			if !resolved.Expose.TLS {
				scheme = "http"
			}
			publicURL = fmt.Sprintf("%s://%s", scheme, resolved.Expose.Domain)
		}
	}

	// ── 9. Persist ────────────────────────────────────────────────────────────
	rawJSON, _ := json.Marshal(raw)
	if err := h.db.Model(&models.Service{}).Where("id = ?", svc.ID).Updates(map[string]any{
		"name":          svc.Name,
		"status":        models.ServiceStatusRunning,
		"manifest_json": string(rawJSON),
		"public_url":    publicURL,
	}).Error; err != nil {
		h.log.Error("jobs", fmt.Sprintf("failed to persist service %s", svc.ID), err)
	}

	h.log.Info("jobs", fmt.Sprintf("service deploy complete: id=%s name=%s", p.ServiceID, svc.Name))
	return nil
}

func (h *ServiceJobHandler) HandleServiceRedeploy(ctx context.Context, t *asynq.Task) error {
	var p ServiceRedeployPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal redeploy payload: %w", err)
	}

	var svc models.Service
	if err := h.db.First(&svc, "id = ?", p.ServiceID).Error; err != nil {
		return fmt.Errorf("service not found: %w", err)
	}
	if svc.ManifestJSON == "" {
		return fmt.Errorf("no manifest stored for service %q", svc.Name)
	}

	h.db.Model(&svc).Update("status", models.ServiceStatusDeploying)

	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		h.markFailed(&svc, err)
		return fmt.Errorf("unmarshal manifest: %w", err)
	}
	if p.ImageOverride != "" {
		raw.Spec.Container.Image = p.ImageOverride
		raw.Spec.Container.Build = nil
	}

	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		h.markFailed(&svc, err)
		return fmt.Errorf("resolve: %w", err)
	}

	if resolved.Build != nil {
		h.log.Info("jobs", fmt.Sprintf("rebuilding image %q for redeploy", resolved.Build.Tag))
		var buildResult *converter.Result
		if resolved.Build.IsGit {
			tarBuf, err := converter.BuildGitContext(resolved.Build.GitURL, resolved.Build.Branch, p.GitToken)
			if err != nil {
				h.markFailed(&svc, err)
				return fmt.Errorf("git clone: %w", err)
			}
			buildResult = &converter.Result{
				BuildRequired:  true,
				BuildTag:       resolved.Build.Tag,
				BuildContext:   tarBuf,
				DockerfilePath: resolved.Build.DockerfilePath,
				GitURL:         resolved.Build.GitURL,
				Branch:         resolved.Build.Branch,
				GitToken:       p.GitToken,
			}
		} else {
			buildResult = &converter.Result{
				BuildRequired:    true,
				BuildTag:         resolved.Build.Tag,
				InlineDockerfile: resolved.Build.DockerfileInline,
				DockerfilePath:   resolved.Build.DockerfilePath,
			}
		}
		if err := h.buildImage(ctx, buildResult); err != nil {
			h.markFailed(&svc, err)
			return fmt.Errorf("rebuild image: %w", err)
		}
	}

	projectNetwork, _ := h.resolveProjectNetwork(svc.ProjectID)
	_ = h.ensureNetwork(ctx, proxyNetwork)
	primaryNetwork := proxyNetwork
	if projectNetwork != "" {
		_ = h.ensureNetwork(ctx, projectNetwork)
		primaryNetwork = projectNetwork
	}

	strategy := resolved.Scaling.Strategy
	switch strategy {
	case "blue-green":
		if err := h.deployBlueGreen(ctx, &svc, resolved); err != nil {
			h.markFailed(&svc, err)
			return fmt.Errorf("blue-green deploy: %w", err)
		}
	default:
		h.removeContainers(ctx, svc.Name)
		isPodman := h.rt.Type() == runtime.RuntimePodman
		spec := manifest.ToContainerSpec(resolved, primaryNetwork, isPodman)
		spec.Labels["tidefly.service-id"] = svc.ID.String()
		spec.Labels["tidefly.service"] = svc.Name
		spec.Labels["tidefly.project"] = svc.ProjectID
		if err := h.rt.CreateContainer(ctx, spec); err != nil {
			h.markFailed(&svc, err)
			return fmt.Errorf("create container: %w", err)
		}
		if raw.Spec.Expose != nil && primaryNetwork != proxyNetwork {
			if err := h.rt.ConnectNetwork(ctx, svc.Name, proxyNetwork); err != nil {
				h.log.Warnw("jobs", "failed to connect to proxy network", "service", svc.Name, "error", err)
			}
		}
	}

	updatedJSON, _ := json.Marshal(&raw)
	h.db.Model(&svc).Updates(map[string]any{
		"status":        models.ServiceStatusRunning,
		"manifest_json": string(updatedJSON),
	})

	h.log.Info("jobs", fmt.Sprintf("service redeploy complete: %s (strategy=%s)", svc.Name, strategy))
	return nil
}

func (h *ServiceJobHandler) HandleServiceUpdate(ctx context.Context, t *asynq.Task) error {
	var p ServiceUpdatePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal update payload: %w", err)
	}

	var svc models.Service
	if err := h.db.First(&svc, "id = ?", p.ServiceID).Error; err != nil {
		return fmt.Errorf("service not found: %w", err)
	}
	if svc.ManifestJSON == "" {
		return fmt.Errorf("no manifest for service %q", svc.Name)
	}

	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return fmt.Errorf("unmarshal manifest: %w", err)
	}

	changed := false
	if p.Image != "" && p.Image != raw.Spec.Container.Image {
		raw.Spec.Container.Image = p.Image
		raw.Spec.Container.Build = nil
		changed = true
	}
	if p.Domain != "" {
		if raw.Spec.Expose == nil {
			raw.Spec.Expose = &manifest.ExposeSpec{}
		}
		if raw.Spec.Expose.Domain != p.Domain {
			raw.Spec.Expose.Domain = p.Domain
			changed = true
		}
	}
	if p.Replicas > 0 {
		if raw.Spec.Scaling == nil {
			raw.Spec.Scaling = &manifest.ScalingSpec{}
		}
		if raw.Spec.Scaling.Replicas != p.Replicas {
			raw.Spec.Scaling.Replicas = p.Replicas
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if p.Domain != "" {
		resolved, _ := manifest.Resolve(&raw)
		if resolved != nil && resolved.Expose.Domain != "" {
			_ = h.ingress.UpdateRoute(ctx, caddyingress.RouteFromManifest(
				svc.Name, resolved.Expose.Domain, resolved.Expose.Port, resolved.Expose.TLS, resolved.Expose.WWW,
			))
		}
	}

	if p.Image != "" {
		h.removeContainers(ctx, svc.Name)

		projectNetwork, _ := h.resolveProjectNetwork(svc.ProjectID)
		_ = h.ensureNetwork(ctx, proxyNetwork)
		primaryNetwork := proxyNetwork
		if projectNetwork != "" {
			_ = h.ensureNetwork(ctx, projectNetwork)
			primaryNetwork = projectNetwork
		}

		resolved, err := manifest.Resolve(&raw)
		if err != nil {
			return fmt.Errorf("resolve: %w", err)
		}
		isPodman := h.rt.Type() == runtime.RuntimePodman
		spec := manifest.ToContainerSpec(resolved, primaryNetwork, isPodman)
		spec.Labels["tidefly.service-id"] = svc.ID.String()
		spec.Labels["tidefly.service"] = svc.Name
		spec.Labels["tidefly.project"] = svc.ProjectID
		if err := h.rt.CreateContainer(ctx, spec); err != nil {
			return fmt.Errorf("create container: %w", err)
		}
		if raw.Spec.Expose != nil && primaryNetwork != proxyNetwork {
			if err := h.rt.ConnectNetwork(ctx, svc.Name, proxyNetwork); err != nil {
				h.log.Warnw("jobs", "failed to connect to proxy network", "service", svc.Name, "error", err)
			}
		}
	}

	updatedJSON, _ := json.Marshal(&raw)
	h.db.Model(&svc).Updates(map[string]any{
		"status":        models.ServiceStatusRunning,
		"manifest_json": string(updatedJSON),
	})
	return nil
}

func (h *ServiceJobHandler) HandleServiceDelete(ctx context.Context, t *asynq.Task) error {
	var p ServiceDeletePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal delete payload: %w", err)
	}

	var svc models.Service
	if err := h.db.First(&svc, "id = ?", p.ServiceID).Error; err != nil {
		return fmt.Errorf("service not found: %w", err)
	}

	// ── 1. Collect resources before containers are removed ────────────────────
	var images []string
	var volumes []string

	containers, err := h.rt.ListContainers(ctx, true)
	if err == nil {
		for _, ct := range containers {
			if ct.Labels["tidefly.service"] != svc.Name {
				continue
			}
			if ct.Image != "" {
				images = append(images, ct.Image)
			}
			details, err := h.rt.GetContainer(ctx, ct.ID)
			if err == nil {
				for _, m := range details.Mounts {
					if m.Source != "" {
						volumes = append(volumes, m.Source)
					}
				}
			}
		}
	}

	// ── 2. Remove ingress route + containers + DB record ──────────────────────
	_ = h.ingress.RemoveRoute(ctx, svc.Name)
	h.removeContainers(ctx, svc.Name)
	if err := h.db.Delete(&svc).Error; err != nil {
		return err
	}

	// ── 3. Enqueue async cleanup for orphaned images + volumes ────────────────
	if len(images) > 0 || len(volumes) > 0 {
		if err := EnqueueServiceCleanup(h.client, svc.Name, images, volumes); err != nil {
			h.log.Warn("jobs", fmt.Sprintf("failed to enqueue cleanup for service %q: %v", svc.Name, err))
		}
	}

	return nil
}

func (h *ServiceJobHandler) resolveProjectNetwork(projectID string) (string, error) {
	if projectID == "" {
		return "", nil
	}
	var project models.Project
	if err := h.db.First(&project, "id = ?", projectID).Error; err != nil {
		return "", fmt.Errorf("project %q not found: %w", projectID, err)
	}
	return project.NetworkName, nil
}
