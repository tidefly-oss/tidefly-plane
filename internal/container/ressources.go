package container

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type getResourcesInput struct {
	ID string `path:"id"`
}

type getResourcesOutput struct {
	Body *runtime.ResourceConfig
}

type containerAutoscalingConfig struct {
	Enabled bool    `json:"enabled"`
	Min     int     `json:"min" minimum:"1"`
	Max     int     `json:"max" minimum:"1"`
	Metric  string  `json:"metric" enum:"cpu,memory,requests"`
	Target  float64 `json:"target" minimum:"1" maximum:"100"`
}

type updateResourcesInput struct {
	ID   string `path:"id"`
	Body struct {
		CPUCores       float64                     `json:"cpu_cores" minimum:"0"`
		MemoryMB       int64                       `json:"memory_mb" minimum:"0"`
		MemorySwapMB   int64                       `json:"memory_swap_mb" minimum:"-1"`
		RestartPolicy  string                      `json:"restart_policy,omitempty" enum:"no,always,on-failure,unless-stopped"`
		MaxRetries     int                         `json:"max_retries" minimum:"0"`
		Replicas       int                         `json:"replicas,omitempty" minimum:"1"`
		DeployStrategy string                      `json:"deploy_strategy,omitempty" enum:"rolling,recreate,blue-green"`
		Autoscaling    *containerAutoscalingConfig `json:"autoscaling,omitempty"`
	}
}

type updateResourcesOutput struct {
	Body struct {
		RestartRequired bool     `json:"restart_required"`
		Applied         []string `json:"applied"`
		Message         string   `json:"message"`
	}
}

func (h *Handler) getResources(ctx context.Context, input *getResourcesInput) (*getResourcesOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	if err := middleware.CheckContainerAccess(ctx, h.db, details.Labels); err != nil {
		return nil, err
	}
	cfg, err := h.runtime.GetResources(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("get resources: %w", err)
	}
	if serviceID := details.Labels["tidefly.service-id"]; serviceID != "" {
		var svc models.Service
		if err := h.db.WithContext(ctx).Select("manifest_json").Where("id = ?", serviceID).First(&svc).Error; err == nil && svc.ManifestJSON != "" {
			var raw manifest.ServiceManifest
			if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err == nil && raw.Spec.Scaling != nil {
				if raw.Spec.Scaling.Strategy != "" {
					cfg.DeployStrategy = raw.Spec.Scaling.Strategy
				}
				if raw.Spec.Scaling.Replicas > 0 {
					cfg.Replicas = raw.Spec.Scaling.Replicas
				}
				if as := raw.Spec.Scaling.Autoscaling; as != nil {
					cfg.Autoscaling = &runtime.AutoscalingConfig{
						Enabled: as.Enabled, Metric: as.Metric,
						Target: float64(as.Target), Min: as.Min, Max: as.Max,
					}
				}
			}
		}
	}
	return &getResourcesOutput{Body: cfg}, nil
}

func (h *Handler) updateResources(ctx context.Context, input *updateResourcesInput) (*updateResourcesOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	if err := middleware.CheckContainerAccess(ctx, h.db, details.Labels); err != nil {
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
	if input.Body.Autoscaling != nil && input.Body.Autoscaling.Min > input.Body.Autoscaling.Max {
		return nil, huma422("autoscaling min must be <= max")
	}
	cfg := runtime.ResourceConfig{
		CPUCores: input.Body.CPUCores, MemoryMB: input.Body.MemoryMB,
		MemorySwapMB: input.Body.MemorySwapMB, RestartPolicy: input.Body.RestartPolicy,
		MaxRetries: input.Body.MaxRetries, Replicas: input.Body.Replicas,
		DeployStrategy: input.Body.DeployStrategy,
	}
	if input.Body.Autoscaling != nil {
		cfg.Autoscaling = &runtime.AutoscalingConfig{
			Enabled: input.Body.Autoscaling.Enabled, Min: input.Body.Autoscaling.Min,
			Max: input.Body.Autoscaling.Max, Metric: input.Body.Autoscaling.Metric,
			Target: input.Body.Autoscaling.Target,
		}
	}
	result, err := h.runtime.UpdateResources(ctx, input.ID, cfg)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditContainerUpdate,
		ResourceID: input.ID,
		Success:    err == nil,
		Details: fmt.Sprintf("cpu=%.2f mem=%dMB swap=%dMB restart=%s retries=%d replicas=%d strategy=%s",
			input.Body.CPUCores, input.Body.MemoryMB, input.Body.MemorySwapMB,
			input.Body.RestartPolicy, input.Body.MaxRetries, input.Body.Replicas, input.Body.DeployStrategy),
	})
	if err != nil {
		return nil, fmt.Errorf("update resources: %w", err)
	}
	if input.Body.Replicas > 0 || input.Body.Autoscaling != nil || input.Body.DeployStrategy != "" {
		h.syncManifestScaling(details.Name, input)
	}
	out := &updateResourcesOutput{}
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

func (h *Handler) syncManifestScaling(containerName string, input *updateResourcesInput) {
	var svc models.Service
	if err := h.db.Where("name = ?", containerName).First(&svc).Error; err != nil {
		return
	}
	if svc.ManifestJSON == "" {
		return
	}
	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return
	}
	if raw.Spec.Scaling == nil {
		raw.Spec.Scaling = &manifest.ScalingSpec{}
	}
	if input.Body.Replicas > 0 {
		raw.Spec.Scaling.Replicas = input.Body.Replicas
	}
	if input.Body.DeployStrategy != "" {
		raw.Spec.Scaling.Strategy = input.Body.DeployStrategy
	}
	if as := input.Body.Autoscaling; as != nil {
		raw.Spec.Scaling.Autoscaling = &manifest.AutoscalingSpec{
			Enabled: as.Enabled, Metric: as.Metric, Target: int(as.Target),
			Min: as.Min, Max: as.Max,
		}
	}
	updated, err := json.Marshal(&raw)
	if err != nil {
		return
	}
	h.db.Model(&svc).Update("manifest_json", string(updated))
}
