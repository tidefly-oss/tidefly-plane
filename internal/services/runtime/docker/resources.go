package docker

import (
	"context"
	"fmt"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

func (d *Runtime) UpdateResources(
	ctx context.Context,
	containerID string,
	cfg runtime.ResourceConfig,
) (*runtime.UpdateResult, error) {
	info, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	result := &runtime.UpdateResult{}
	needsRestart := false

	currentMemBytes := info.HostConfig.Memory
	newMemBytes := cfg.MemoryMB * 1024 * 1024

	if cfg.MemoryMB > 0 && currentMemBytes > 0 && newMemBytes < currentMemBytes {
		needsRestart = true
	}

	updateConfig := dockercontainer.UpdateConfig{}
	resources := dockercontainer.Resources{}

	if cfg.CPUCores > 0 {
		resources.NanoCPUs = int64(cfg.CPUCores * 1e9)
		result.Applied = append(result.Applied, fmt.Sprintf("cpu=%.2f cores", cfg.CPUCores))
	} else if cfg.CPUCores == 0 {
		resources.NanoCPUs = 0
		result.Applied = append(result.Applied, "cpu=unlimited")
	}

	if cfg.MemoryMB >= 0 {
		resources.Memory = newMemBytes
		if cfg.MemoryMB == 0 {
			result.Applied = append(result.Applied, "memory=unlimited")
		} else {
			result.Applied = append(result.Applied, fmt.Sprintf("memory=%d MB", cfg.MemoryMB))
		}
	}

	if cfg.MemoryMB > 0 {
		switch cfg.MemorySwapMB {
		case -1:
			resources.MemorySwap = -1
			result.Applied = append(result.Applied, "swap=unlimited")
		case 0:
			resources.MemorySwap = newMemBytes
			result.Applied = append(result.Applied, "swap=disabled")
		default:
			resources.MemorySwap = cfg.MemorySwapMB * 1024 * 1024
			result.Applied = append(result.Applied, fmt.Sprintf("swap=%d MB total", cfg.MemorySwapMB))
		}
	} else if cfg.MemorySwapMB == -1 {
		resources.MemorySwap = -1
		result.Applied = append(result.Applied, "swap=unlimited")
	}

	if cfg.RestartPolicy != "" {
		updateConfig.RestartPolicy = dockercontainer.RestartPolicy{
			Name:              dockercontainer.RestartPolicyMode(cfg.RestartPolicy),
			MaximumRetryCount: cfg.MaxRetries,
		}
		result.Applied = append(result.Applied, fmt.Sprintf("restart=%s", cfg.RestartPolicy))
	}

	updateConfig.Resources = resources

	if _, err := d.client.ContainerUpdate(ctx, containerID, updateConfig); err != nil {
		return nil, fmt.Errorf("update container resources: %w", err)
	}

	if needsRestart && info.State.Running {
		if err := d.client.ContainerRestart(
			ctx, containerID, dockercontainer.StopOptions{
				Timeout: new(10),
			},
		); err != nil {
			return nil, fmt.Errorf("restart container after memory reduction: %w", err)
		}
		result.RestartRequired = true
	}

	return result, nil
}

func (d *Runtime) GetResources(ctx context.Context, containerID string) (*runtime.ResourceConfig, error) {
	info, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	hc := info.HostConfig
	cfg := &runtime.ResourceConfig{}
	if hc.NanoCPUs > 0 {
		cfg.CPUCores = float64(hc.NanoCPUs) / 1e9
	}
	if hc.Memory > 0 {
		cfg.MemoryMB = hc.Memory / (1024 * 1024)
	}
	if hc.MemorySwap == -1 {
		cfg.MemorySwapMB = -1
	} else if hc.MemorySwap > 0 {
		cfg.MemorySwapMB = hc.MemorySwap / (1024 * 1024)
	}

	cfg.RestartPolicy = string(hc.RestartPolicy.Name)
	cfg.MaxRetries = hc.RestartPolicy.MaximumRetryCount

	return cfg, nil
}
