package reconciler

import (
	"context"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

// rolling performs a zero-downtime rolling update:
// For each running replica: start new → wait healthy → stop old.
//
// If a new replica fails the health check, the rollout is aborted
// and old replicas keep running. The Reconciler retries on the next tick.
func (r *Reconciler) rolling(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest) error {
	containers, err := r.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("rolling: list containers: %w", err)
	}

	var current []runtime.Container
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == svc.Name {
			current = append(current, ct)
		}
	}

	desiredReplicas := resolved.Scaling.Replicas
	if desiredReplicas < 1 {
		desiredReplicas = 1
	}

	isPodman := r.rt.Type() == runtime.RuntimePodman

	// Nothing running → simple deploy
	if len(current) == 0 {
		spec := manifest.ToContainerSpec(resolved, proxyNetwork, isPodman)
		spec.Labels["tidefly.service-id"] = svc.ID.String()
		spec.Labels["tidefly.service"] = svc.Name
		return r.rt.CreateContainer(ctx, spec)
	}

	// Roll each replica one by one
	for i, old := range current {
		newName := fmt.Sprintf("%s-roll-%d-%d", svc.Name, time.Now().Unix(), i)

		newResolved := *resolved
		newResolved.Name = newName

		spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
		spec.Labels["tidefly.service-id"] = svc.ID.String()
		spec.Labels["tidefly.service"] = svc.Name
		spec.Labels["tidefly.rolling-index"] = fmt.Sprintf("%d", i)

		r.log.Info("reconciler", fmt.Sprintf("rolling: %q replica %d/%d starting (%s)", svc.Name, i+1, len(current), newName))

		if err := r.rt.CreateContainer(ctx, spec); err != nil {
			return fmt.Errorf("rolling: create replica %d: %w", i+1, err)
		}

		if err := waitHealthy(ctx, r.rt, newName, healthTimeout); err != nil {
			// Abort — remove failed new replica, keep old running
			_ = r.rt.StopContainer(ctx, newName, runtime.StopOptions{})
			_ = r.rt.DeleteContainer(ctx, newName, true)
			return fmt.Errorf("rolling: replica %d/%d unhealthy, rollout aborted (old kept): %w", i+1, len(current), err)
		}

		r.log.Info("reconciler", fmt.Sprintf("rolling: %q replica %d/%d healthy — stopping old (%s)", svc.Name, i+1, len(current), old.Name))

		// Only stop old after new is confirmed healthy
		_ = r.rt.StopContainer(ctx, old.ID, runtime.StopOptions{})
		_ = r.rt.DeleteContainer(ctx, old.ID, true)
	}

	// Scale up to desired if needed
	for i := len(current); i < desiredReplicas; i++ {
		newName := fmt.Sprintf("%s-%d", svc.Name, i+1)
		newResolved := *resolved
		newResolved.Name = newName

		spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
		spec.Labels["tidefly.service-id"] = svc.ID.String()
		spec.Labels["tidefly.service"] = svc.Name

		if err := r.rt.CreateContainer(ctx, spec); err != nil {
			r.log.Error("reconciler", fmt.Sprintf("rolling: scale-up replica %d failed", i+1), err)
		}
	}

	r.log.Info("reconciler", fmt.Sprintf("rolling: %q complete (%d replicas)", svc.Name, desiredReplicas))
	return nil
}
