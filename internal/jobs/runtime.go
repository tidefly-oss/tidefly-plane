package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

func (h *Handler) HandleRuntimeCleanup(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		OlderThanHours    float64 `json:"older_than_hours"`
		StoppedContainers bool    `json:"stopped_containers"`
		DanglingImages    bool    `json:"dangling_images"`
		UnusedVolumes     bool    `json:"unused_volumes"`
	}
	_ = json.Unmarshal(t.Payload(), &payload)

	h.log.Info("jobs", fmt.Sprintf("runtime cleanup job started (runtime: %s)", h.rt.Type()))

	allContainers, err := h.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("cleanup: list containers: %w", err)
	}

	usedImages := make(map[string]string)
	usedVolumes := make(map[string]string)

	for _, c := range allContainers {
		usedImages[c.Image] = c.Name
		details, err := h.rt.GetContainer(ctx, c.ID)
		if err != nil {
			continue
		}
		for _, m := range details.Mounts {
			if m.Source != "" {
				usedVolumes[m.Source] = c.Name
			}
		}
	}

	var removed, errors int
	cutoff := time.Now().Add(-time.Duration(payload.OlderThanHours * float64(time.Hour)))

	// ── Stopped Containers ───────────────────────────────────────────────────
	if payload.StoppedContainers {
		for _, c := range allContainers {
			if c.Status == runtime.StatusRunning || c.Status == runtime.StatusPaused {
				continue
			}
			if c.Created.After(cutoff) {
				continue
			}
			if err := h.rt.DeleteContainer(ctx, c.ID, false); err != nil {
				h.log.Warn("jobs", fmt.Sprintf("cleanup: could not remove container %s: %v", c.Name, err))
				errors++
				continue
			}
			removed++
			h.log.Info("jobs", fmt.Sprintf("cleanup: removed stopped container %s (%s)", c.Name, c.ID))
		}
	}

	// ── Dangling Images ──────────────────────────────────────────────────────
	if payload.DanglingImages {
		images, err := h.rt.ListImages(ctx)
		if err != nil {
			return fmt.Errorf("cleanup: list images: %w", err)
		}
		for _, img := range images {
			if len(img.Tags) > 0 {
				continue
			}
			if img.Created.After(cutoff) {
				continue
			}
			if containerName, inUse := usedImages[img.ID]; inUse {
				h.log.Info("jobs", fmt.Sprintf("cleanup: skip image %s — in use by %s", img.ID[:12], containerName))
				continue
			}
			if err := h.rt.DeleteImage(ctx, img.ID, false); err != nil {
				h.log.Warn("jobs", fmt.Sprintf("cleanup: could not remove image %s: %v", img.ID[:12], err))
				errors++
				continue
			}
			removed++
			h.log.Info("jobs", fmt.Sprintf("cleanup: removed dangling image %s", img.ID[:12]))
		}
	}

	// ── Unused Volumes ───────────────────────────────────────────────────────
	if payload.UnusedVolumes {
		volumes, err := h.rt.ListVolumes(ctx)
		if err != nil {
			return fmt.Errorf("cleanup: list volumes: %w", err)
		}
		for _, v := range volumes {
			if v.CreatedAt.After(cutoff) {
				continue
			}
			if containerName, inUse := usedVolumes[v.Mountpath]; inUse {
				h.log.Info("jobs", fmt.Sprintf("cleanup: skip volume %s — in use by %s", v.Name, containerName))
				continue
			}
			if err := h.rt.DeleteVolume(ctx, v.Name); err != nil {
				// skip volumes still mounted by a container
				if strings.Contains(err.Error(), "volume is in use") ||
					strings.Contains(err.Error(), "status 409") {
					continue
				}
				h.log.Warn("jobs", fmt.Sprintf("cleanup: could not remove volume %s: %v", v.Name, err))
				errors++
				continue
			}
			removed++
			h.log.Info("jobs", fmt.Sprintf("cleanup: removed unused volume %s", v.Name))
		}
	}

	if errors > 0 {
		h.log.Warn("jobs", fmt.Sprintf("runtime cleanup finished: removed %d resources, %d errors", removed, errors))
	} else {
		h.log.Info("jobs", fmt.Sprintf("runtime cleanup finished: removed %d resources", removed))
	}
	return nil
}

func (h *Handler) HandleRuntimeHealthCheck(ctx context.Context, _ *asynq.Task) error {
	containers, err := h.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("healthcheck: list containers: %w", err)
	}
	for _, c := range containers {
		if c.Status == runtime.StatusExited {
			if _, err := h.rt.GetContainer(ctx, c.ID); err != nil {
				continue
			}
			h.log.ContainerEvent(
				"WARN", c.ID, c.Name,
				"Container exited unexpectedly",
				fmt.Sprintf("container %s (image: %s) is in exited state", c.Name, c.Image),
			)
		}
	}
	return nil
}
