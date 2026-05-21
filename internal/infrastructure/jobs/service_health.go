package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

func (h *ServiceJobHandler) HandleServiceHeal(ctx context.Context, t *asynq.Task) error {
	var p ServiceHealPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal heal payload: %w", err)
	}

	var svc models.Service
	if err := h.db.Where("name = ? AND manifest_service = ?", p.ServiceName, true).
		First(&svc).Error; err != nil {
		return nil
	}

	h.log.Info("jobs", fmt.Sprintf("self-heal: triggered for %q (reason=%s)", p.ServiceName, p.Reason))

	if svc.ManifestJSON == "" ||
		svc.Status == models.ServiceStatusDeploying ||
		svc.Status == models.ServiceStatusStopped ||
		svc.Status == models.ServiceStatusRestarting {
		h.log.Info("jobs", fmt.Sprintf("self-heal: skipping %q (status=%s)", p.ServiceName, svc.Status))
		return nil
	}

	time.Sleep(2 * time.Second)

	// Worker node heal
	if svc.WorkerID != "" && h.agentClient != nil && h.agentClient.IsConnected(svc.WorkerID) {
		var raw manifest.ServiceManifest
		if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
			return fmt.Errorf("unmarshal manifest: %w", err)
		}
		resolved, err := manifest.Resolve(&raw)
		if err != nil {
			return fmt.Errorf("resolve: %w", err)
		}
		deployCmd := resolvedToDeployCmd(&svc, resolved)
		if err := h.agentClient.SendHeal(ctx, svc.WorkerID, svc.Name, p.Reason, deployCmd); err != nil {
			h.log.Error("jobs", fmt.Sprintf("worker self-heal failed for %q", p.ServiceName), err)
			h.db.Model(&svc).Update("status", models.ServiceStatusFailed)
			return fmt.Errorf("worker self-heal %q: %w", p.ServiceName, err)
		}
		h.db.Model(&svc).Update("status", models.ServiceStatusRunning)
		h.log.Info("jobs", fmt.Sprintf("self-heal: %q recovered on worker %s", p.ServiceName, svc.WorkerID))
		return nil
	}

	// Local heal
	containers, err := h.rt.ListContainers(ctx, true)
	if err == nil {
		for _, ct := range containers {
			if ct.Labels["tidefly.service"] == p.ServiceName && !runtime.NeedsRestart(ct.Status) {
				h.log.Info("jobs", fmt.Sprintf("self-heal: %q already recovered — skipping", p.ServiceName))
				return nil
			}
		}
	}

	if err := h.restartService(ctx, &svc); err != nil {
		h.log.Error("jobs", fmt.Sprintf("self-heal failed for %q", p.ServiceName), err)
		h.db.Model(&svc).Update("status", models.ServiceStatusFailed)
		return fmt.Errorf("self-heal %q: %w", p.ServiceName, err)
	}

	h.db.Model(&svc).Update("status", models.ServiceStatusRunning)
	h.log.Info("jobs", fmt.Sprintf("self-heal: %q recovered (reason=%s)", p.ServiceName, p.Reason))
	return nil
}

func (h *ServiceJobHandler) HandleServiceHealthCheck(ctx context.Context, _ *asynq.Task) error {
	var services []models.Service
	if err := h.db.Where("manifest_service = ? AND status NOT IN ?", true, []models.ServiceStatus{
		models.ServiceStatusStopped,
		models.ServiceStatusRestarting,
	}).Find(&services).Error; err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	if len(services) == 0 {
		return nil
	}

	containers, err := h.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	running := make(map[string]runtime.ContainerStatus)
	for _, ct := range containers {
		if name := ct.Labels["tidefly.service"]; name != "" {
			running[name] = ct.Status
		}
	}

	healed := 0
	for i := range services {
		svc := &services[i]

		if svc.Name == "" || svc.ManifestJSON == "" {
			_, hasContainer := running[svc.Name]
			if !hasContainer {
				h.log.Info("jobs", fmt.Sprintf("self-heal: purging orphaned service id=%s name=%q", svc.ID, svc.Name))
				h.db.Delete(svc)
				continue
			}
			h.log.Warn("jobs", fmt.Sprintf("self-heal: skipping orphaned service id=%s name=%q (has container)", svc.ID, svc.Name))
			continue
		}

		if svc.Status == models.ServiceStatusDeploying {
			_, hasContainer := running[svc.Name]
			if !hasContainer && time.Since(svc.UpdatedAt) > 10*time.Minute {
				h.log.Info("jobs", fmt.Sprintf("self-heal: purging stuck deploying service %q", svc.Name))
				h.db.Delete(svc)
				continue
			}
			continue
		}

		if svc.Status == models.ServiceStatusRestarting {
			continue
		}

		// Worker node — health managed by agent
		if svc.WorkerID != "" {
			if h.agentClient != nil && !h.agentClient.IsConnected(svc.WorkerID) {
				h.log.Warn("jobs", fmt.Sprintf("self-heal: worker %s offline for service %q", svc.WorkerID, svc.Name))
			}
			continue
		}

		// Local
		status, exists := running[svc.Name]
		if exists && !runtime.NeedsRestart(status) {
			if svc.Status != models.ServiceStatusRunning {
				h.db.Model(svc).Update("status", models.ServiceStatusRunning)
			}
			continue
		}

		h.log.Info("jobs", fmt.Sprintf("self-heal fallback: restarting %q (exists=%v status=%v)", svc.Name, exists, status))
		if err := h.restartService(ctx, svc); err != nil {
			h.log.Error("jobs", fmt.Sprintf("self-heal fallback failed for %q", svc.Name), err)
			h.db.Model(svc).Update("status", models.ServiceStatusFailed)
			continue
		}
		h.db.Model(svc).Update("status", models.ServiceStatusRunning)
		healed++
	}

	if healed > 0 {
		h.log.Info("jobs", fmt.Sprintf("self-heal fallback: %d service(s) restarted", healed))
	}
	return nil
}

func (h *ServiceJobHandler) restartService(ctx context.Context, svc *models.Service) error {
	if svc.ManifestJSON == "" {
		return fmt.Errorf("no manifest stored")
	}
	h.removeContainers(ctx, svc.Name)

	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return fmt.Errorf("unmarshal manifest: %w", err)
	}
	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	_ = h.ensureNetwork(ctx, proxyNetwork)

	isPodman := h.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(resolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()

	time.Sleep(2 * time.Second)
	return h.rt.CreateContainer(ctx, spec)
}
