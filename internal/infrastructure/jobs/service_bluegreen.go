package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

func (h *ServiceJobHandler) deployBlueGreen(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest) error {
	// Worker node blue-green
	if svc.WorkerID != "" && h.agentClient != nil && h.agentClient.IsConnected(svc.WorkerID) {
		deployCmd := resolvedToDeployCmd(svc, resolved)
		err := h.agentClient.SendBlueGreen(
			ctx,
			svc.WorkerID,
			svc.Name,
			svc.ActiveSlotName,
			resolved.Expose.Domain,
			int32(resolved.Expose.Port),
			resolved.Expose.TLS,
			deployCmd,
		)
		if err != nil {
			return fmt.Errorf("worker blue-green deploy: %w", err)
		}
		newSlot := "green"
		if svc.ActiveSlotName == "green" {
			newSlot = "blue"
		}
		h.db.Model(svc).Update("active_slot", newSlot)
		h.log.Info("jobs", fmt.Sprintf("blue-green: worker deploy complete for %q slot=%s", svc.Name, newSlot))
		return nil
	}

	// Local blue-green
	isPodman := h.rt.Type() == runtime.RuntimePodman

	currentSlot := svc.ActiveSlotName
	newSlot := "green"
	if currentSlot == "green" {
		newSlot = "blue"
	}

	oldName := svc.Name
	if currentSlot != "" {
		oldName = fmt.Sprintf("%s-%s", svc.Name, currentSlot)
	}
	newName := fmt.Sprintf("%s-%s", svc.Name, newSlot)

	h.log.Info("jobs", fmt.Sprintf("blue-green: starting %q slot=%s", newName, newSlot))

	newResolved := *resolved
	newResolved.Name = newName

	spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.slot"] = newSlot

	if err := h.rt.CreateContainer(ctx, spec); err != nil {
		return fmt.Errorf("create %s container: %w", newSlot, err)
	}

	h.log.Info("jobs", fmt.Sprintf("blue-green: waiting for %q to become healthy", newName))
	if err := h.waitHealthy(ctx, newName, 60*time.Second); err != nil {
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

	containers, _ := h.rt.ListContainers(ctx, true)
	for _, ct := range containers {
		isOldByName := ct.Name == oldName
		isOldByLabel := ct.Labels["tidefly.service"] == svc.Name && ct.Labels["tidefly.slot"] == currentSlot
		if isOldByName || isOldByLabel {
			_ = h.rt.StopContainer(ctx, ct.ID, runtime.StopOptions{})
			_ = h.rt.DeleteContainer(ctx, ct.ID, true)
			h.log.Info("jobs", fmt.Sprintf("blue-green: removed old container %q", ct.Name))
		}
	}

	h.db.Model(svc).Update("active_slot", newSlot)
	h.log.Info("jobs", fmt.Sprintf("blue-green: deploy complete — active slot is now %q", newSlot))
	return nil
}

func (h *ServiceJobHandler) waitHealthy(ctx context.Context, containerName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		containers, err := h.rt.ListContainers(ctx, true)
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}
		for _, ct := range containers {
			if ct.Name == containerName {
				switch ct.Status {
				case runtime.StatusRunning:
					return nil
				case runtime.StatusExited, runtime.StatusDead:
					return fmt.Errorf("container %q exited during startup (status=%s)", containerName, ct.Status)
				}
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
