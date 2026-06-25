package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"
)

const (
	loopInterval     = 30 * time.Second
	healDelay        = 2 * time.Second
	healthTimeout    = 60 * time.Second
	stuckDeployAfter = 10 * time.Minute
	proxyNetwork     = "tidefly_proxy"
)

type Action string

const (
	ActionNoop      Action = "noop"
	ActionHeal      Action = "heal"
	ActionScaleUp   Action = "scale_up"
	ActionScaleDown Action = "scale_down"
	ActionUpdate    Action = "update"
	ActionRedeploy  Action = "redeploy"
)

type Delta struct {
	ServiceID   string
	ServiceName string
	Action      Action
	Reason      string
}

// Reconciler is the Kubernetes-style control loop.
// Every 30s it compares desired state (ManifestJSON) against
// actual state (running containers) and acts on any delta.
type Reconciler struct {
	db      *gorm.DB
	rt      runtime.Runtime
	ingress ingress.Adapter
	notif   *notification.Service
	log     *applogger.Logger
}

func New(
	db *gorm.DB,
	rt runtime.Runtime,
	ing ingress.Adapter,
	notif *notification.Service,
	log *applogger.Logger,
) *Reconciler {
	return &Reconciler{db: db, rt: rt, ingress: ing, notif: notif, log: log}
}

// Run starts the reconcile loop — blocks until ctx cancelled.
func (r *Reconciler) Run(ctx context.Context) error {
	r.log.Info("reconciler", "starting (interval: 30s)")
	ticker := time.NewTicker(loopInterval)
	defer ticker.Stop()

	r.reconcileAll(ctx) // run immediately on start

	for {
		select {
		case <-ctx.Done():
			r.log.Info("reconciler", "stopping")
			return nil
		case <-ticker.C:
			r.reconcileAll(ctx)
		}
	}
}

func (r *Reconciler) reconcileAll(ctx context.Context) {
	var services []models.Service
	if err := r.db.WithContext(ctx).
		Where("manifest_service = ? AND status NOT IN ?", true, []models.ServiceStatus{
			models.ServiceStatusStopped,
			models.ServiceStatusDeploying,
		}).Find(&services).Error; err != nil {
		r.log.Error("reconciler", "list services failed", err)
		return
	}
	if len(services) == 0 {
		return
	}

	containers, err := r.rt.ListContainers(ctx, true)
	if err != nil {
		r.log.Error("reconciler", "list containers failed", err)
		return
	}
	byService := indexByService(containers)

	for i := range services {
		svc := &services[i]
		if svc.WorkerID != "" {
			continue // remote worker handles its own reconcile
		}
		r.reconcileOne(ctx, svc, byService)
	}
}

func (r *Reconciler) reconcileOne(ctx context.Context, svc *models.Service, byService map[string][]runtime.Container) {
	if svc.ManifestJSON == "" {
		return
	}
	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		r.log.Error("reconciler", fmt.Sprintf("unmarshal manifest for %q", svc.Name), err)
		return
	}
	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		r.log.Error("reconciler", fmt.Sprintf("resolve manifest for %q", svc.Name), err)
		return
	}

	actual := byService[svc.Name]
	delta := r.diff(svc, resolved, actual)
	if delta.Action == ActionNoop {
		return
	}

	r.log.Info("reconciler", fmt.Sprintf("service=%q action=%s reason=%s", svc.Name, delta.Action, delta.Reason))

	switch delta.Action {
	case ActionHeal:
		r.heal(ctx, svc, resolved)
	case ActionScaleUp:
		r.scaleUp(ctx, svc, resolved, len(actual))
	case ActionScaleDown:
		r.scaleDown(ctx, svc, actual, len(actual))
	case ActionUpdate:
		r.update(ctx, svc, resolved)
	case ActionRedeploy:
		r.redeploy(ctx, svc, resolved)
	}
}

// diff computes desired vs actual state and returns the action to take.
func (r *Reconciler) diff(svc *models.Service, resolved *manifest.ResolvedManifest, actual []runtime.Container) Delta {
	base := Delta{ServiceID: svc.ID.String(), ServiceName: svc.Name}

	desiredReplicas := resolved.Scaling.Replicas
	if desiredReplicas < 1 {
		desiredReplicas = 1
	}

	running := countRunning(actual)

	// 1. No containers at all → heal
	if running == 0 {
		return Delta{Action: ActionHeal, Reason: "no running containers", ServiceID: base.ServiceID, ServiceName: base.ServiceName}
	}

	// 2. OCI image digest drift → update (blue-green or rolling)
	if svc.HasImageDrift() {
		return Delta{
			Action:    ActionUpdate,
			Reason:    fmt.Sprintf("image digest changed (%s → %s)", shortDigest(svc.DeployedDigest), shortDigest(svc.RemoteDigest)),
			ServiceID: base.ServiceID, ServiceName: base.ServiceName,
		}
	}

	// 3. Replica count drift → scale
	if running < desiredReplicas {
		return Delta{
			Action:    ActionScaleUp,
			Reason:    fmt.Sprintf("replicas %d < desired %d", running, desiredReplicas),
			ServiceID: base.ServiceID, ServiceName: base.ServiceName,
		}
	}
	if running > desiredReplicas {
		return Delta{
			Action:    ActionScaleDown,
			Reason:    fmt.Sprintf("replicas %d > desired %d", running, desiredReplicas),
			ServiceID: base.ServiceID, ServiceName: base.ServiceName,
		}
	}

	// 4. Stuck in deploying → heal
	if svc.Status == models.ServiceStatusDeploying && time.Since(svc.UpdatedAt) > stuckDeployAfter {
		return Delta{Action: ActionHeal, Reason: "stuck deploying >10m", ServiceID: base.ServiceID, ServiceName: base.ServiceName}
	}

	return Delta{Action: ActionNoop, ServiceID: base.ServiceID, ServiceName: base.ServiceName}
}

// ── Actions ───────────────────────────────────────────────────────────────────

func (r *Reconciler) heal(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest) {
	time.Sleep(healDelay)

	// Re-check — might have recovered already
	containers, _ := r.rt.ListContainers(ctx, true)
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == svc.Name && !runtime.NeedsRestart(ct.Status) {
			r.log.Info("reconciler", fmt.Sprintf("heal: %q already recovered", svc.Name))
			r.db.Model(svc).Update("status", models.ServiceStatusRunning)
			return
		}
	}

	r.removeContainers(ctx, svc.Name)

	isPodman := r.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(resolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.service"] = svc.Name

	if err := r.rt.CreateContainer(ctx, spec); err != nil {
		r.log.Error("reconciler", fmt.Sprintf("heal: create failed for %q", svc.Name), err)
		r.db.Model(svc).Updates(map[string]any{"status": models.ServiceStatusFailed, "last_error": err.Error()})
		r.notify(ctx, svc.Name, models.SeverityError, fmt.Sprintf("self-heal FAILED for %q: %s — manual intervention required", svc.Name, err.Error()))
		return
	}

	r.db.Model(svc).Updates(map[string]any{"status": models.ServiceStatusRunning, "last_error": ""})
	r.log.Info("reconciler", fmt.Sprintf("heal: %q recovered", svc.Name))
	r.notify(ctx, svc.Name, models.SeverityInfo, fmt.Sprintf("service %q recovered automatically", svc.Name))
}

func (r *Reconciler) scaleUp(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest, current int) {
	newName := fmt.Sprintf("%s-%d", svc.Name, current+1)
	newResolved := *resolved
	newResolved.Name = newName

	isPodman := r.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.service"] = svc.Name

	r.log.Info("reconciler", fmt.Sprintf("scale up: %s %d→%d", svc.Name, current, current+1))
	if err := r.rt.CreateContainer(ctx, spec); err != nil {
		r.log.Error("reconciler", fmt.Sprintf("scale up failed for %q", svc.Name), err)
	}
}

func (r *Reconciler) scaleDown(ctx context.Context, _ *models.Service, containers []runtime.Container, current int) {
	// Remove highest-named replica first (newest)
	var toRemove *runtime.Container
	for i := range containers {
		ct := &containers[i]
		if toRemove == nil || ct.Name > toRemove.Name {
			toRemove = ct
		}
	}
	if toRemove == nil {
		return
	}
	r.log.Info("reconciler", fmt.Sprintf("scale down: removing %s (%d→%d)", toRemove.Name, current, current-1))
	_ = r.rt.StopContainer(ctx, toRemove.ID, runtime.StopOptions{})
	_ = r.rt.DeleteContainer(ctx, toRemove.ID, true)
}

func (r *Reconciler) update(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest) {
	r.log.Info("reconciler", fmt.Sprintf("update: %q strategy=%s", svc.Name, resolved.Scaling.Strategy))
	r.db.Model(svc).Update("status", models.ServiceStatusDeploying)

	var err error
	switch resolved.Scaling.Strategy {
	case "blue-green":
		err = r.blueGreen(ctx, svc, resolved)
	default: // rolling, recreate, or empty
		err = r.rolling(ctx, svc, resolved)
	}

	if err != nil {
		r.log.Error("reconciler", fmt.Sprintf("update failed for %q", svc.Name), err)
		r.db.Model(svc).Updates(map[string]any{"status": models.ServiceStatusFailed, "last_error": err.Error()})
		return
	}

	r.db.Model(svc).Updates(map[string]any{
		"status":           models.ServiceStatusRunning,
		"deployed_digest":  svc.RemoteDigest,
		"update_available": false,
		"update_source":    "",
		"last_error":       "",
	})
	r.notify(ctx, svc.Name, models.SeverityInfo, fmt.Sprintf("service %q updated to latest image", svc.Name))
}

func (r *Reconciler) redeploy(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest) {
	r.log.Info("reconciler", fmt.Sprintf("redeploy: %q (config drift)", svc.Name))
	r.db.Model(svc).Update("status", models.ServiceStatusDeploying)
	r.removeContainers(ctx, svc.Name)

	isPodman := r.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(resolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.service"] = svc.Name

	if err := r.rt.CreateContainer(ctx, spec); err != nil {
		r.log.Error("reconciler", fmt.Sprintf("redeploy failed for %q", svc.Name), err)
		r.db.Model(svc).Updates(map[string]any{"status": models.ServiceStatusFailed, "last_error": err.Error()})
		return
	}
	r.db.Model(svc).Updates(map[string]any{"status": models.ServiceStatusRunning, "last_error": ""})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (r *Reconciler) removeContainers(ctx context.Context, serviceName string) {
	containers, err := r.rt.ListContainers(ctx, true)
	if err != nil {
		return
	}
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == serviceName || ct.Name == serviceName {
			_ = r.rt.StopContainer(ctx, ct.ID, runtime.StopOptions{})
			_ = r.rt.DeleteContainer(ctx, ct.ID, true)
		}
	}
}

func (r *Reconciler) notify(ctx context.Context, serviceName string, severity models.NotificationSeverity, msg string) {
	if r.notif == nil {
		return
	}
	go func() {
		nCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.notif.Upsert(nCtx, "reconciler:"+serviceName, serviceName, severity, msg)
	}()
}

func indexByService(containers []runtime.Container) map[string][]runtime.Container {
	m := make(map[string][]runtime.Container)
	for _, ct := range containers {
		if name := ct.Labels["tidefly.service"]; name != "" {
			m[name] = append(m[name], ct)
		}
	}
	return m
}

func countRunning(containers []runtime.Container) int {
	n := 0
	for _, ct := range containers {
		if ct.Status == runtime.StatusRunning {
			n++
		}
	}
	return n
}

func waitHealthy(ctx context.Context, rt runtime.Runtime, containerName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		containers, err := rt.ListContainers(ctx, true)
		if err != nil {
			return err
		}
		for _, ct := range containers {
			if ct.Name != containerName {
				continue
			}
			switch ct.Status {
			case runtime.StatusRunning:
				return nil
			case runtime.StatusExited, runtime.StatusDead:
				return fmt.Errorf("container %q exited during startup", containerName)
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

func shortDigest(d string) string {
	if len(d) > 19 {
		return d[7:19] + "…"
	}
	return d
}
