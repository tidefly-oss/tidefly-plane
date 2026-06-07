package podman

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (p *Runtime) GetResources(ctx context.Context, containerID string) (*runtime.ResourceConfig, error) {
	var inspect struct {
		HostConfig *struct {
			NanoCpus      *int64  `json:"NanoCpus"`
			CPUQuota      *int64  `json:"CpuQuota"`
			CPUPeriod     *uint64 `json:"CpuPeriod"`
			Memory        *int64  `json:"Memory"`
			MemorySwap    *int64  `json:"MemorySwap"`
			RestartPolicy *struct {
				Name              *string `json:"Name"`
				MaximumRetryCount *int    `json:"MaximumRetryCount"`
			} `json:"RestartPolicy"`
		} `json:"HostConfig"`
	}

	code, err := p.c.getJSON(ctx, "/libpod/containers/"+escPath(containerID)+"/json", nil, &inspect)
	if err != nil {
		return nil, fmt.Errorf("podman inspect for resources: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("podman inspect for resources: status %d", code)
	}

	cfg := &runtime.ResourceConfig{}
	hc := inspect.HostConfig
	if hc == nil {
		return cfg, nil
	}

	// ── Podman-native fields ──────────────────────────────────────────────────
	if hc.NanoCpus != nil && *hc.NanoCpus > 0 {
		cfg.CPUCores = float64(*hc.NanoCpus) / 1e9
	} else if hc.CPUQuota != nil && *hc.CPUQuota > 0 {
		period := int64(100_000)
		if hc.CPUPeriod != nil && *hc.CPUPeriod > 0 {
			period = int64(*hc.CPUPeriod)
		}
		cfg.CPUCores = float64(*hc.CPUQuota) / float64(period)
	}

	if hc.Memory != nil && *hc.Memory > 0 {
		cfg.MemoryMB = *hc.Memory / (1024 * 1024)
	}
	if hc.MemorySwap != nil {
		switch {
		case *hc.MemorySwap == -1:
			cfg.MemorySwapMB = -1
		case *hc.MemorySwap > 0:
			cfg.MemorySwapMB = *hc.MemorySwap / (1024 * 1024)
		}
	}
	if hc.RestartPolicy != nil {
		cfg.RestartPolicy = derefStr(hc.RestartPolicy.Name)
		if hc.RestartPolicy.MaximumRetryCount != nil {
			cfg.MaxRetries = *hc.RestartPolicy.MaximumRetryCount
		}
	}

	// ── Tidefly-specific fields from ContainerMeta ───────────────────────────
	if p.db != nil {
		var meta models.ContainerMeta
		if err := p.db.WithContext(ctx).
			Where("container_id = ?", containerID).
			First(&meta).Error; err == nil {
			cfg.DeployStrategy = meta.DeployStrategy
			cfg.Replicas = meta.Replicas
			if meta.AutoscalingEnabled {
				cfg.Autoscaling = &runtime.AutoscalingConfig{Enabled: true}
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			_ = err
		}
	}

	// Defaults
	if cfg.DeployStrategy == "" {
		cfg.DeployStrategy = "rolling"
	}
	if cfg.Replicas == 0 {
		cfg.Replicas = 1
	}

	return cfg, nil
}

func (p *Runtime) UpdateResources(
	ctx context.Context, containerID string, cfg runtime.ResourceConfig,
) (*runtime.UpdateResult, error) {
	current, err := p.GetResources(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("podman inspect for update: %w", err)
	}

	result := &runtime.UpdateResult{}
	needsRestart := false

	newMemBytes := cfg.MemoryMB * 1024 * 1024
	if cfg.MemoryMB > 0 && current.MemoryMB > 0 && cfg.MemoryMB < current.MemoryMB {
		needsRestart = true
	}

	// ── Podman-native update ──────────────────────────────────────────────────
	type linuxCPU struct {
		Period *uint64 `json:"period,omitempty"`
		Quota  *int64  `json:"quota,omitempty"`
	}
	type linuxMemory struct {
		Limit *int64 `json:"limit,omitempty"`
		Swap  *int64 `json:"swap,omitempty"`
	}
	type updateBody struct {
		CPU    *linuxCPU    `json:"cpu,omitempty"`
		Memory *linuxMemory `json:"memory,omitempty"`
	}

	body := updateBody{}

	if cfg.CPUCores > 0 {
		body.CPU = &linuxCPU{Period: new(uint64(100_000)), Quota: new(int64(cfg.CPUCores * 100_000))}
		result.Applied = append(result.Applied, fmt.Sprintf("cpu=%.2f cores", cfg.CPUCores))
	} else if cfg.CPUCores == 0 {
		body.CPU = &linuxCPU{Quota: new(int64(-1))}
		result.Applied = append(result.Applied, "cpu=unlimited")
	}

	if cfg.MemoryMB >= 0 {
		mem := &linuxMemory{}
		if cfg.MemoryMB == 0 {
			mem.Limit = new(int64(-1))
			result.Applied = append(result.Applied, "memory=unlimited")
		} else {
			mem.Limit = &newMemBytes
			result.Applied = append(result.Applied, fmt.Sprintf("memory=%d MB", cfg.MemoryMB))
		}
		if cfg.MemoryMB > 0 {
			switch cfg.MemorySwapMB {
			case -1:
				mem.Swap = new(int64(-1))
				result.Applied = append(result.Applied, "swap=unlimited")
			case 0:
				mem.Swap = &newMemBytes
				result.Applied = append(result.Applied, "swap=disabled")
			default:
				mem.Swap = new(cfg.MemorySwapMB * 1024 * 1024)
				result.Applied = append(result.Applied, fmt.Sprintf("swap=%d MB total", cfg.MemorySwapMB))
			}
		}
		body.Memory = mem
	}

	q := url.Values{}
	if cfg.RestartPolicy != "" {
		q.Set("restartPolicy", cfg.RestartPolicy)
		if cfg.RestartPolicy == "on-failure" && cfg.MaxRetries > 0 {
			q.Set("restartRetries", fmt.Sprintf("%d", cfg.MaxRetries))
		}
		result.Applied = append(result.Applied, fmt.Sprintf("restart=%s", cfg.RestartPolicy))
	}

	code, _, err := p.c.post(ctx, "/libpod/containers/"+escPath(containerID)+"/update", q, body)
	if err != nil {
		return nil, fmt.Errorf("podman update resources: %w", err)
	}
	if code != 200 && code != 201 {
		return nil, fmt.Errorf("podman update resources: status %d", code)
	}

	// ── Tidefly-specific fields → ContainerMeta (upsert) ─────────────────────
	if p.db != nil {
		meta := models.ContainerMeta{
			ContainerID:        containerID,
			AutoscalingEnabled: cfg.Autoscaling != nil && cfg.Autoscaling.Enabled,
			Replicas:           cfg.Replicas,
		}
		if meta.Replicas == 0 {
			meta.Replicas = 1
		}
		if cfg.DeployStrategy != "" {
			meta.DeployStrategy = cfg.DeployStrategy
		} else {
			meta.DeployStrategy = "rolling"
		}
		if cfg.Autoscaling != nil && cfg.Autoscaling.Enabled {
			result.Applied = append(result.Applied, "autoscaling=on")
		}
		if cfg.DeployStrategy != "" {
			result.Applied = append(result.Applied, fmt.Sprintf("strategy=%s", cfg.DeployStrategy))
		}
		if err := p.db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "container_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"deploy_strategy", "autoscaling_enabled", "replicas", "updated_at"}),
			}).
			Create(&meta).Error; err != nil {
			_ = err
		}
	}

	// ── Restart if memory was reduced ─────────────────────────────────────────
	if needsRestart {
		running, _ := p.isRunning(ctx, containerID)
		if running {
			restartCode, _, restartErr := p.c.post(
				ctx, "/libpod/containers/"+escPath(containerID)+"/restart",
				url.Values{"t": {"10"}}, nil,
			)
			if restartErr != nil {
				return nil, fmt.Errorf("podman restart after memory reduction: %w", restartErr)
			}
			if restartCode != 204 {
				return nil, fmt.Errorf("podman restart after memory reduction: status %d", restartCode)
			}
			result.RestartRequired = true
		}
	}

	return result, nil
}
