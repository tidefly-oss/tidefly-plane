package http

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

type GetResourcesInput struct {
	ID string `path:"id"`
}
type GetResourcesOutput struct {
	Body *runtime.ResourceConfig
}

type UpdateResourcesInput struct {
	ID   string `path:"id"`
	Body struct {
		CPUCores      float64 `json:"cpu_cores" minimum:"0"`
		MemoryMB      int64   `json:"memory_mb" minimum:"0"`
		MemorySwapMB  int64   `json:"memory_swap_mb" minimum:"-1"`
		RestartPolicy string  `json:"restart_policy,omitempty" enum:"no,always,on-failure,unless-stopped"`
		MaxRetries    int     `json:"max_retries" minimum:"0"`
	}
}
type UpdateResourcesOutput struct {
	Body struct {
		RestartRequired bool     `json:"restart_required"`
		Applied         []string `json:"applied"`
		Message         string   `json:"message"`
	}
}

func (h *Handler) GetResources(ctx context.Context, input *GetResourcesInput) (*GetResourcesOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	if err := middleware.CheckContainerAccessHuma(ctx, h.db, details.Labels); err != nil {
		return nil, err
	}
	cfg, err := h.runtime.GetResources(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("get resources: %w", err)
	}
	return &GetResourcesOutput{Body: cfg}, nil
}

func (h *Handler) UpdateResources(ctx context.Context, input *UpdateResourcesInput) (*UpdateResourcesOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	if err := middleware.CheckContainerAccessHuma(ctx, h.db, details.Labels); err != nil {
		return nil, err
	}
	if input.Body.MemoryMB > 0 && input.Body.MemoryMB < 6 {
		return nil, huma422("memory_mb must be >= 6 or 0 (unlimited)")
	}
	if input.Body.MemoryMB > 0 && input.Body.MemorySwapMB > 0 && input.Body.MemorySwapMB < input.Body.MemoryMB {
		return nil, huma422(fmt.Sprintf("memory_swap_mb must be >= memory_mb (%d)", input.Body.MemoryMB))
	}
	if input.Body.MaxRetries > 0 && input.Body.RestartPolicy != "on-failure" {
		return nil, huma422("max_retries only valid with restart_policy=on-failure")
	}
	cfg := runtime.ResourceConfig{
		CPUCores:      input.Body.CPUCores,
		MemoryMB:      input.Body.MemoryMB,
		MemorySwapMB:  input.Body.MemorySwapMB,
		RestartPolicy: input.Body.RestartPolicy,
		MaxRetries:    input.Body.MaxRetries,
	}
	result, err := h.runtime.UpdateResources(ctx, input.ID, cfg)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditContainerUpdate,
			ResourceID: input.ID,
			Success:    err == nil,
			Details: fmt.Sprintf(
				"cpu=%.2f mem=%dMB swap=%dMB restart=%s retries=%d",
				input.Body.CPUCores, input.Body.MemoryMB, input.Body.MemorySwapMB,
				input.Body.RestartPolicy, input.Body.MaxRetries,
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("update resources: %w", err)
	}
	out := &UpdateResourcesOutput{}
	out.Body.RestartRequired = result.RestartRequired
	out.Body.Applied = result.Applied
	switch {
	case result.RestartRequired:
		out.Body.Message = "Memory limit reduced — container was restarted to apply changes"
	case len(result.Applied) == 0:
		out.Body.Message = "No changes applied"
	default:
		out.Body.Message = "Resource limits updated live"
	}
	return out, nil
}
