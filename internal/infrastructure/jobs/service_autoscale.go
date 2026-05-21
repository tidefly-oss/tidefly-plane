package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

const (
	scaleUpCooldown   = 30 * time.Second
	scaleDownCooldown = 3 * time.Minute
)

var scaleHistory = &scaleTracker{m: make(map[string]scaleEntry)}

type scaleEntry struct {
	lastScaleUp         time.Time
	lastScaleDown       time.Time
	belowThresholdSince time.Time
}

type scaleTracker struct {
	mu sync.Mutex
	m  map[string]scaleEntry
}

func (t *scaleTracker) get(name string) scaleEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.m[name]
}

func (t *scaleTracker) set(name string, e scaleEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[name] = e
}

func (h *ServiceJobHandler) HandleServiceAutoscale(ctx context.Context, _ *asynq.Task) error {
	var services []models.Service
	if err := h.db.Where("manifest_service = ? AND status = ?", true, models.ServiceStatusRunning).
		Find(&services).Error; err != nil {
		return fmt.Errorf("autoscale: list services: %w", err)
	}

	for i := range services {
		if err := h.processAutoscale(ctx, &services[i]); err != nil {
			h.log.Warn("jobs", fmt.Sprintf("autoscale failed for %q: %v", services[i].Name, err))
		}
	}
	return nil
}

func (h *ServiceJobHandler) processAutoscale(ctx context.Context, svc *models.Service) error {
	if svc.ManifestJSON == "" {
		return nil
	}

	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return nil
	}

	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		return nil
	}

	as := resolved.Scaling.Autoscaling
	if !as.Enabled {
		return nil
	}

	now := time.Now()
	entry := scaleHistory.get(svc.Name)

	// Worker node autoscale
	if svc.WorkerID != "" && h.agentClient != nil && h.agentClient.IsConnected(svc.WorkerID) {
		return h.processWorkerAutoscale(ctx, svc, resolved, as, now, entry)
	}

	// Local autoscale
	containers, err := h.rt.ListContainers(ctx, false)
	if err != nil {
		return err
	}

	var svContainers []runtime.Container
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == svc.Name {
			svContainers = append(svContainers, ct)
		}
	}
	if len(svContainers) == 0 {
		return nil
	}

	var totalCPU, totalMem float64
	var measured int
	for _, ct := range svContainers {
		m, err := h.readStats(ctx, ct.ID)
		if err != nil {
			continue
		}
		totalCPU += m.cpuPercent
		totalMem += m.memPercent
		measured++
	}
	if measured == 0 {
		return nil
	}

	avgCPU := totalCPU / float64(measured)
	avgMem := totalMem / float64(measured)
	target := float64(as.Target)
	current := len(svContainers)

	switch {
	case (avgCPU >= target || avgMem >= target) && current < as.Max:
		if now.Sub(entry.lastScaleUp) < scaleUpCooldown {
			return nil
		}
		if err := h.scaleUp(ctx, svc, resolved, current); err != nil {
			return err
		}
		entry.lastScaleUp = now
		entry.belowThresholdSince = time.Time{}
		scaleHistory.set(svc.Name, entry)

	case avgCPU < target*0.5 && avgMem < target*0.5 && current > as.Min:
		if entry.belowThresholdSince.IsZero() {
			entry.belowThresholdSince = now
			scaleHistory.set(svc.Name, entry)
			return nil
		}
		if now.Sub(entry.belowThresholdSince) < scaleDownCooldown {
			return nil
		}
		if err := h.scaleDown(ctx, svc, svContainers, current); err != nil {
			return err
		}
		entry.lastScaleDown = now
		entry.belowThresholdSince = time.Time{}
		scaleHistory.set(svc.Name, entry)

	default:
		if !entry.belowThresholdSince.IsZero() && avgCPU >= target*0.5 {
			entry.belowThresholdSince = time.Time{}
			scaleHistory.set(svc.Name, entry)
		}
	}

	return nil
}

func (h *ServiceJobHandler) processWorkerAutoscale(
	ctx context.Context,
	svc *models.Service,
	resolved *manifest.ResolvedManifest,
	as manifest.ResolvedAutoscaling,
	now time.Time,
	entry scaleEntry,
) error {
	// Get container list from worker to count replicas
	workerContainers, err := h.agentClient.ListContainers(ctx, svc.WorkerID)
	if err != nil {
		return fmt.Errorf("list worker containers: %w", err)
	}

	var current int32
	for _, ct := range workerContainers {
		if ct.Labels["tidefly.service"] == svc.Name {
			current++
		}
	}
	if current == 0 {
		return nil
	}

	// Collect metrics from worker
	metrics, err := h.agentClient.CollectMetrics(ctx, svc.WorkerID)
	if err != nil {
		return fmt.Errorf("collect worker metrics: %w", err)
	}

	avgCPU := metrics.CpuPercent
	avgMem := metrics.MemUsedMb / metrics.MemTotalMb * 100
	target := float64(as.Target)
	deployCmd := resolvedToDeployCmd(svc, resolved)

	switch {
	case (avgCPU >= target || avgMem >= target) && current < int32(as.Max):
		if now.Sub(entry.lastScaleUp) < scaleUpCooldown {
			return nil
		}
		h.log.Info("jobs", fmt.Sprintf("worker autoscale UP: %s %d→%d", svc.Name, current, current+1))
		if err := h.agentClient.SendAutoscale(ctx, svc.WorkerID, svc.Name, current, current+1, deployCmd); err != nil {
			return err
		}
		entry.lastScaleUp = now
		entry.belowThresholdSince = time.Time{}
		scaleHistory.set(svc.Name, entry)

	case avgCPU < target*0.5 && avgMem < target*0.5 && current > int32(as.Min):
		if entry.belowThresholdSince.IsZero() {
			entry.belowThresholdSince = now
			scaleHistory.set(svc.Name, entry)
			return nil
		}
		if now.Sub(entry.belowThresholdSince) < scaleDownCooldown {
			return nil
		}
		h.log.Info("jobs", fmt.Sprintf("worker autoscale DOWN: %s %d→%d", svc.Name, current, current-1))
		if err := h.agentClient.SendAutoscale(ctx, svc.WorkerID, svc.Name, current, current-1, deployCmd); err != nil {
			return err
		}
		entry.lastScaleDown = now
		entry.belowThresholdSince = time.Time{}
		scaleHistory.set(svc.Name, entry)
	}

	return nil
}

func (h *ServiceJobHandler) scaleUp(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest, current int) error {
	newName := fmt.Sprintf("%s-%d", svc.Name, current+1)
	newResolved := *resolved
	newResolved.Name = newName

	isPodman := h.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()

	h.log.Info("jobs", fmt.Sprintf("autoscale UP: %s %d→%d", svc.Name, current, current+1))
	return h.rt.CreateContainer(ctx, spec)
}

func (h *ServiceJobHandler) scaleDown(ctx context.Context, svc *models.Service, containers []runtime.Container, current int) error {
	var toRemove *runtime.Container
	for i := range containers {
		ct := &containers[i]
		if ct.Name == svc.Name {
			continue
		}
		if toRemove == nil || ct.Name > toRemove.Name {
			toRemove = ct
		}
	}
	if toRemove == nil {
		return nil
	}

	h.log.Info("jobs", fmt.Sprintf("autoscale DOWN: %s %d→%d (removing %s)", svc.Name, current, current-1, toRemove.Name))
	_ = h.rt.StopContainer(ctx, toRemove.ID, runtime.StopOptions{})
	return h.rt.DeleteContainer(ctx, toRemove.ID, true)
}
