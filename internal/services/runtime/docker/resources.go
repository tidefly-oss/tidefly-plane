package docker

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	dockercontainer "github.com/docker/docker/api/types/container"
)

// UpdateResources implementiert die Hybrid-Strategie:
//
//	CPU erhöhen/verringern  → live (docker update)
//	RAM erhöhen             → live (docker update)
//	RAM verringern          → Warnung + Restart
//	Swap ändern             → live wenn RAM nicht verringert, sonst im Restart
//	RestartPolicy           → immer live (docker update)
func (d *Runtime) UpdateResources(ctx context.Context, containerID string, cfg runtime.ResourceConfig) (*runtime.UpdateResult, error) {
	// Aktuelle Container-Infos holen um zu vergleichen
	info, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	result := &runtime.UpdateResult{}
	needsRestart := false

	// ── RAM-Verringerung erkennen ──────────────────────────────────────────────
	currentMemBytes := info.HostConfig.Resources.Memory // 0 = unlimited
	newMemBytes := cfg.MemoryMB * 1024 * 1024

	if cfg.MemoryMB > 0 && currentMemBytes > 0 && newMemBytes < currentMemBytes {
		// RAM wird verringert → Restart nötig
		needsRestart = true
	}

	// ── Update-Request zusammenbauen ──────────────────────────────────────────
	updateConfig := dockercontainer.UpdateConfig{}
	resources := dockercontainer.Resources{}

	// CPU
	if cfg.CPUCores > 0 {
		resources.NanoCPUs = int64(cfg.CPUCores * 1e9)
		result.Applied = append(result.Applied, fmt.Sprintf("cpu=%.2f cores", cfg.CPUCores))
	} else if cfg.CPUCores == 0 {
		// Explizit auf 0 = unlimited
		resources.NanoCPUs = 0
		result.Applied = append(result.Applied, "cpu=unlimited")
	}

	// Memory
	if cfg.MemoryMB >= 0 {
		resources.Memory = newMemBytes
		if cfg.MemoryMB == 0 {
			result.Applied = append(result.Applied, "memory=unlimited")
		} else {
			result.Applied = append(result.Applied, fmt.Sprintf("memory=%d MB", cfg.MemoryMB))
		}
	}

	// Memory Swap
	// Docker: MemorySwap = RAM + Swap zusammen. -1 = unlimited.
	// Wichtig: Wenn Memory gesetzt wird, muss MemorySwap immer mitgeschickt werden,
	// sonst bleibt der alte Swap-Wert und ist evtl. größer als das neue Memory-Limit.
	if cfg.MemoryMB > 0 {
		switch cfg.MemorySwapMB {
		case -1:
			// Explizit unlimited Swap
			resources.MemorySwap = -1
			result.Applied = append(result.Applied, "swap=unlimited")
		case 0:
			// Kein Swap: MemorySwap = Memory (kein extra Swap, nur RAM)
			resources.MemorySwap = newMemBytes
			result.Applied = append(result.Applied, "swap=disabled")
		default:
			// Expliziter Swap-Wert — muss >= Memory sein (Validierung im Handler)
			resources.MemorySwap = cfg.MemorySwapMB * 1024 * 1024
			result.Applied = append(result.Applied, fmt.Sprintf("swap=%d MB total", cfg.MemorySwapMB))
		}
	} else if cfg.MemorySwapMB == -1 {
		// Memory unlimited aber Swap explizit auf unlimited setzen
		resources.MemorySwap = -1
		result.Applied = append(result.Applied, "swap=unlimited")
	}

	// Restart Policy
	if cfg.RestartPolicy != "" {
		updateConfig.RestartPolicy = dockercontainer.RestartPolicy{
			Name:              dockercontainer.RestartPolicyMode(cfg.RestartPolicy),
			MaximumRetryCount: cfg.MaxRetries,
		}
		result.Applied = append(result.Applied, fmt.Sprintf("restart=%s", cfg.RestartPolicy))
	}

	updateConfig.Resources = resources

	// ── Docker Update aufrufen ─────────────────────────────────────────────────
	if _, err := d.client.ContainerUpdate(ctx, containerID, updateConfig); err != nil {
		return nil, fmt.Errorf("update container resources: %w", err)
	}

	// ── Restart wenn RAM verringert ────────────────────────────────────────────
	if needsRestart && info.State.Running {
		timeout := 10 // Sekunden
		if err := d.client.ContainerRestart(ctx, containerID, dockercontainer.StopOptions{
			Timeout: &timeout,
		}); err != nil {
			return nil, fmt.Errorf("restart container after memory reduction: %w", err)
		}
		result.RestartRequired = true
	}

	return result, nil
}

// GetResources liest die aktuellen Resource-Limits eines Containers.
// Wird verwendet um die Defaults im Frontend vorzubefüllen.
func (d *Runtime) GetResources(ctx context.Context, containerID string) (*runtime.ResourceConfig, error) {
	info, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	hc := info.HostConfig
	cfg := &runtime.ResourceConfig{}

	// CPU: NanoCPUs → Cores (NanoCPUs lives in HostConfig.Resources)
	if hc.Resources.NanoCPUs > 0 {
		cfg.CPUCores = float64(hc.Resources.NanoCPUs) / 1e9
	}

	// Memory: Bytes → MB
	if hc.Resources.Memory > 0 {
		cfg.MemoryMB = hc.Resources.Memory / (1024 * 1024)
	}

	// Swap
	if hc.Resources.MemorySwap == -1 {
		cfg.MemorySwapMB = -1
	} else if hc.Resources.MemorySwap > 0 {
		cfg.MemorySwapMB = hc.Resources.MemorySwap / (1024 * 1024)
	}

	// Restart Policy
	cfg.RestartPolicy = string(hc.RestartPolicy.Name)
	cfg.MaxRetries = hc.RestartPolicy.MaximumRetryCount

	return cfg, nil
}
