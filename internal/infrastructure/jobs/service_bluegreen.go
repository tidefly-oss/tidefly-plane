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

// deployBlueGreen performs a zero-downtime deploy by:
//  1. Starting a new container in the inactive slot (blue/green)
//  2. Waiting for it to become healthy
//  3. Switching the Caddy route to the new container
//  4. Removing the old container
func (h *ServiceJobHandler) deployBlueGreen(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest) error {
	isPodman := h.rt.Type() == runtime.RuntimePodman

	// ── 1. Determine slots ────────────────────────────────────────────────────
	currentSlot := svc.ActiveSlotName // "" | "blue" | "green"
	newSlot := "green"
	if currentSlot == "green" {
		newSlot = "blue"
	}

	// On the very first blue-green deploy the container has no slot suffix
	oldName := svc.Name
	if currentSlot != "" {
		oldName = fmt.Sprintf("%s-%s", svc.Name, currentSlot)
	}
	newName := fmt.Sprintf("%s-%s", svc.Name, newSlot)

	h.log.Info("jobs", fmt.Sprintf("blue-green: starting %q slot=%s", newName, newSlot))

	// ── 2. Start new container ────────────────────────────────────────────────
	newResolved := *resolved
	newResolved.Name = newName

	spec := manifest.ToContainerSpec(&newResolved, "tidefly_proxy", isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.slot"] = newSlot

	if err := h.rt.CreateContainer(ctx, spec); err != nil {
		return fmt.Errorf("create %s container: %w", newSlot, err)
	}

	// ── 3. Wait for healthy ───────────────────────────────────────────────────
	h.log.Info("jobs", fmt.Sprintf("blue-green: waiting for %q to become healthy", newName))
	if err := h.waitHealthy(ctx, newName, 60*time.Second); err != nil {
		// New container unhealthy — roll back, old container keeps traffic
		_ = h.rt.StopContainer(ctx, newName, runtime.StopOptions{})
		_ = h.rt.DeleteContainer(ctx, newName, true)
		return fmt.Errorf("new container %q unhealthy, rolled back: %w", newName, err)
	}

	// ── 4. Switch Caddy route ─────────────────────────────────────────────────
	if resolved.Expose.Domain != "" {
		route := ingress.Route{
			ServiceName: svc.Name, // keep route ID stable across slots
			Domain:      resolved.Expose.Domain,
			Upstream:    fmt.Sprintf("%s:%d", newName, resolved.Expose.Port),
			TLS:         resolved.Expose.TLS,
			WWW:         resolved.Expose.WWW,
		}
		if err := h.ingress.UpdateRoute(ctx, route); err != nil {
			// Route switch failed — roll back
			_ = h.rt.StopContainer(ctx, newName, runtime.StopOptions{})
			_ = h.rt.DeleteContainer(ctx, newName, true)
			return fmt.Errorf("switch caddy route to %q failed, rolled back: %w", newName, err)
		}
		h.log.Info("jobs", fmt.Sprintf("blue-green: traffic switched to %q", newName))
	}

	// ── 5. Remove old container ───────────────────────────────────────────────
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

	// ── 6. Persist active slot ────────────────────────────────────────────────
	h.db.Model(svc).Update("active_slot", newSlot)

	h.log.Info("jobs", fmt.Sprintf("blue-green: deploy complete — active slot is now %q", newSlot))
	return nil
}

// waitHealthy polls the container status every 2s until it is running,
// or until the timeout expires.
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
