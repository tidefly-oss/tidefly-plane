package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/queue"
)

func (h *ServiceJobHandler) HandleServiceCleanup(ctx context.Context, t *asynq.Task) error {
	var p queue.ServiceCleanupPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal cleanup payload: %w", err)
	}

	h.log.Info("jobs", fmt.Sprintf("service cleanup started: service=%s images=%d volumes=%d",
		p.ServiceName, len(p.Images), len(p.Volumes)))

	allContainers, err := h.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("cleanup: list containers: %w", err)
	}

	usedImages := make(map[string]struct{})
	usedVolumes := make(map[string]struct{})

	for _, ct := range allContainers {
		if ct.Labels["tidefly.service"] == p.ServiceName {
			continue
		}
		usedImages[ct.Image] = struct{}{}
		details, err := h.rt.GetContainer(ctx, ct.ID)
		if err != nil {
			continue
		}
		for _, m := range details.Mounts {
			if m.Source != "" {
				usedVolumes[m.Source] = struct{}{}
			}
		}
	}

	for _, vol := range p.Volumes {
		if _, inUse := usedVolumes[vol]; inUse {
			h.log.Info("jobs", fmt.Sprintf("cleanup: skip volume %q — still in use", vol))
			continue
		}
		if err := h.rt.DeleteVolume(ctx, vol); err != nil {
			if strings.Contains(err.Error(), "volume is in use") || strings.Contains(err.Error(), "409") {
				h.log.Info("jobs", fmt.Sprintf("cleanup: skip volume %q — in use (409)", vol))
				continue
			}
			h.log.Warn("jobs", fmt.Sprintf("cleanup: failed to delete volume %q: %v", vol, err))
			continue
		}
		h.log.Info("jobs", fmt.Sprintf("cleanup: deleted volume %q", vol))
	}

	for _, img := range p.Images {
		if _, inUse := usedImages[img]; inUse {
			h.log.Info("jobs", fmt.Sprintf("cleanup: skip image %q — still in use", img))
			continue
		}
		if err := h.rt.DeleteImage(ctx, img, false); err != nil {
			h.log.Warn("jobs", fmt.Sprintf("cleanup: failed to delete image %q: %v", img, err))
			continue
		}
		h.log.Info("jobs", fmt.Sprintf("cleanup: deleted image %q", img))
	}

	h.log.Info("jobs", fmt.Sprintf("service cleanup complete: service=%s", p.ServiceName))
	return nil
}
