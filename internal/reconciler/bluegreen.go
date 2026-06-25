package reconciler

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// blueGreen performs a zero-downtime blue-green deploy:
// 1. Start new slot container
// 2. Wait for it to be healthy
// 3. Switch Caddy traffic to new slot
// 4. Remove old slot
// On any failure after step 1, the new slot is removed and old slot remains active.
func (r *Reconciler) blueGreen(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest) error {
	currentSlot := svc.ActiveSlotName
	newSlot := nextSlot(currentSlot)

	oldName := svc.Name
	if currentSlot != "" {
		oldName = fmt.Sprintf("%s-%s", svc.Name, currentSlot)
	}
	newName := fmt.Sprintf("%s-%s", svc.Name, newSlot)

	r.log.Info("reconciler", fmt.Sprintf("blue-green: %q starting new slot=%s", svc.Name, newSlot))

	newResolved := *resolved
	newResolved.Name = newName

	isPodman := r.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.service"] = svc.Name
	spec.Labels["tidefly.slot"] = newSlot

	if err := r.rt.CreateContainer(ctx, spec); err != nil {
		return fmt.Errorf("blue-green: create %s container: %w", newSlot, err)
	}

	if err := waitHealthy(ctx, r.rt, newName, healthTimeout); err != nil {
		_ = r.rt.StopContainer(ctx, newName, runtime.StopOptions{})
		_ = r.rt.DeleteContainer(ctx, newName, true)
		return fmt.Errorf("blue-green: new slot %q unhealthy, rolled back: %w", newName, err)
	}

	r.log.Info("reconciler", fmt.Sprintf("blue-green: %q slot=%s healthy — switching traffic", svc.Name, newSlot))

	if resolved.Expose.Domain != "" {
		route := ingress.Route{
			ServiceName: svc.Name,
			Domain:      resolved.Expose.Domain,
			Upstream:    fmt.Sprintf("%s:%d", newName, resolved.Expose.Port),
			TLS:         resolved.Expose.TLS,
			WWW:         resolved.Expose.WWW,
		}
		if err := r.ingress.UpdateRoute(ctx, route); err != nil {
			_ = r.rt.StopContainer(ctx, newName, runtime.StopOptions{})
			_ = r.rt.DeleteContainer(ctx, newName, true)
			return fmt.Errorf("blue-green: caddy route switch failed, rolled back: %w", err)
		}
	}

	// Remove old slot after traffic switched
	if containers, err := r.rt.ListContainers(ctx, true); err == nil {
		for _, ct := range containers {
			isOld := ct.Name == oldName ||
				(ct.Labels["tidefly.service"] == svc.Name && ct.Labels["tidefly.slot"] == currentSlot)
			if isOld {
				_ = r.rt.StopContainer(ctx, ct.ID, runtime.StopOptions{})
				_ = r.rt.DeleteContainer(ctx, ct.ID, true)
				r.log.Info("reconciler", fmt.Sprintf("blue-green: removed old slot %q", ct.Name))
			}
		}
	}

	r.db.Model(svc).Update("active_slot", newSlot)
	r.log.Info("reconciler", fmt.Sprintf("blue-green: %q complete — active slot=%s", svc.Name, newSlot))
	return nil
}

func nextSlot(current string) string {
	if current == "green" {
		return "blue"
	}
	return "green"
}
