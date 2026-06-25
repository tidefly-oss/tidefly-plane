package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// deployBlueGreen is a standalone function (not a method) so both
// RedeployWorker and future ReconcilerWorker can call it without
// coupling to a specific handler struct.
func deployBlueGreen(ctx context.Context, h *ServiceWorkers, svc *models.Service, resolved *manifest.ResolvedManifest) error {
	// Remote worker path
	if svc.WorkerID != "" && h.agentClient != nil && h.agentClient.IsConnected(svc.WorkerID) {
		deployCmd := resolvedToDeployCmd(svc, resolved)
		if err := h.agentClient.SendBlueGreen(
			ctx, svc.WorkerID, svc.Name, svc.ActiveSlotName,
			resolved.Expose.Domain, int32(resolved.Expose.Port), resolved.Expose.TLS, deployCmd,
		); err != nil {
			return fmt.Errorf("worker blue-green deploy: %w", err)
		}
		newSlot := nextSlot(svc.ActiveSlotName)
		h.db.Model(svc).Update("active_slot", newSlot)
		h.log.Info("jobs", fmt.Sprintf("blue-green: worker deploy complete for %q slot=%s", svc.Name, newSlot))
		return nil
	}

	// Local path
	currentSlot := svc.ActiveSlotName
	newSlot := nextSlot(currentSlot)

	oldName := svc.Name
	if currentSlot != "" {
		oldName = fmt.Sprintf("%s-%s", svc.Name, currentSlot)
	}
	newName := fmt.Sprintf("%s-%s", svc.Name, newSlot)

	h.log.Info("jobs", fmt.Sprintf("blue-green: starting %q slot=%s", newName, newSlot))

	newResolved := *resolved
	newResolved.Name = newName

	isPodman := h.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.slot"] = newSlot

	if err := h.rt.CreateContainer(ctx, spec); err != nil {
		return fmt.Errorf("create %s container: %w", newSlot, err)
	}

	if err := waitHealthy(ctx, h.rt, newName, 60*time.Second); err != nil {
		_ = h.rt.StopContainer(ctx, newName, runtime.StopOptions{})
		_ = h.rt.DeleteContainer(ctx, newName, true)
		return fmt.Errorf("new container %q unhealthy, rolled back: %w", newName, err)
	}

	if resolved.Expose.Domain != "" {
		route := ingress.Route{
			ServiceName: svc.Name,
			Domain:      resolved.Expose.Domain,
			Upstream:    fmt.Sprintf("%s:%d", newName, resolved.Expose.Port),
			TLS:         resolved.Expose.TLS,
			WWW:         resolved.Expose.WWW,
		}
		if err := h.ingress.UpdateRoute(ctx, route); err != nil {
			_ = h.rt.StopContainer(ctx, newName, runtime.StopOptions{})
			_ = h.rt.DeleteContainer(ctx, newName, true)
			return fmt.Errorf("switch caddy route to %q failed, rolled back: %w", newName, err)
		}
		h.log.Info("jobs", fmt.Sprintf("blue-green: traffic switched to %q", newName))
	}

	// Remove old slot containers
	if containers, err := h.rt.ListContainers(ctx, true); err == nil {
		for _, ct := range containers {
			isOldByName := ct.Name == oldName
			isOldByLabel := ct.Labels["tidefly.service"] == svc.Name && ct.Labels["tidefly.slot"] == currentSlot
			if isOldByName || isOldByLabel {
				_ = h.rt.StopContainer(ctx, ct.ID, runtime.StopOptions{})
				_ = h.rt.DeleteContainer(ctx, ct.ID, true)
				h.log.Info("jobs", fmt.Sprintf("blue-green: removed old container %q", ct.Name))
			}
		}
	}

	h.db.Model(svc).Update("active_slot", newSlot)
	h.log.Info("jobs", fmt.Sprintf("blue-green: deploy complete — active slot is now %q", newSlot))
	return nil
}

func nextSlot(current string) string {
	if current == "green" {
		return "blue"
	}
	return "green"
}

func waitHealthy(ctx context.Context, rt runtime.Runtime, containerName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		containers, err := rt.ListContainers(ctx, true)
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}
		for _, ct := range containers {
			if ct.Name != containerName {
				continue
			}
			switch ct.Status {
			case runtime.StatusRunning:
				return nil
			case runtime.StatusExited, runtime.StatusDead:
				return fmt.Errorf("container %q exited during startup (status=%s)", containerName, ct.Status)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("container %q did not become healthy within %s", containerName, timeout)
}

// ── Container Stats ───────────────────────────────────────────────────────────

type containerMetrics struct {
	cpuPercent float64
	memPercent float64
}

func readContainerStats(ctx context.Context, rt runtime.Runtime, id string) (*containerMetrics, error) {
	rc, err := rt.ContainerStats(ctx, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	data := make([]byte, 65536)
	n, _ := rc.Read(data)
	data = data[:n]

	// Try Docker stats format first
	var ds struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     int    `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
			Stats struct {
				Cache uint64 `json:"cache"`
			} `json:"stats"`
		} `json:"memory_stats"`
	}
	if err := json.Unmarshal(data, &ds); err == nil && ds.CPUStats.SystemCPUUsage > 0 {
		cpuDelta := float64(ds.CPUStats.CPUUsage.TotalUsage) - float64(ds.PreCPUStats.CPUUsage.TotalUsage)
		sysDelta := float64(ds.CPUStats.SystemCPUUsage) - float64(ds.PreCPUStats.SystemCPUUsage)
		cpus := ds.CPUStats.OnlineCPUs
		if cpus == 0 {
			cpus = 1
		}
		var cpu float64
		if sysDelta > 0 {
			cpu = (cpuDelta / sysDelta) * float64(cpus) * 100
		}
		var mem float64
		if ds.MemoryStats.Limit > 0 {
			mem = float64(ds.MemoryStats.Usage-ds.MemoryStats.Stats.Cache) / float64(ds.MemoryStats.Limit) * 100
		}
		return &containerMetrics{cpuPercent: cpu, memPercent: mem}, nil
	}

	// Fallback: Podman stats format
	var ps struct {
		CPU     float64 `json:"CPU"`
		MemPerc float64 `json:"MemPerc"`
	}
	if err := json.Unmarshal(data, &ps); err == nil {
		return &containerMetrics{cpuPercent: ps.CPU, memPercent: ps.MemPerc}, nil
	}

	return nil, fmt.Errorf("unknown stats format")
}
