package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/agent"
	agentpb "github.com/tidefly-oss/tidefly-plane/internal/agent/proto"
	"github.com/tidefly-oss/tidefly-plane/internal/converter"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	caddyingress "github.com/tidefly-oss/tidefly-plane/internal/infra/ingress/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"gorm.io/gorm"
)

const proxyNetwork = "tidefly_proxy"

// ── Job Arg Types ─────────────────────────────────────────────────────────────

type DeployArgs struct {
	ServiceID    string `json:"service_id"`
	ManifestJSON string `json:"manifest_json,omitempty"`
	Image        string `json:"image,omitempty"`
	ComposeYAML  string `json:"compose_yaml,omitempty"`
	Dockerfile   string `json:"dockerfile,omitempty"`
	GitURL       string `json:"git_url,omitempty"`
	GitToken     string `json:"git_token,omitempty"`
	Name         string `json:"name,omitempty"`
	StackName    string `json:"stack_name,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Port         int    `json:"port,omitempty"`
	Expose       bool   `json:"expose,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Replicas     int    `json:"replicas,omitempty"`
	Strategy     string `json:"strategy,omitempty"`
}

func (DeployArgs) Kind() string { return "service:deploy" }

type RedeployArgs struct {
	ServiceID     string `json:"service_id"`
	ImageOverride string `json:"image_override,omitempty"`
	GitToken      string `json:"git_token,omitempty"`
}

func (RedeployArgs) Kind() string { return "service:redeploy" }

type UpdateArgs struct {
	ServiceID string `json:"service_id"`
	Image     string `json:"image,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Replicas  int    `json:"replicas,omitempty"`
}

func (UpdateArgs) Kind() string { return "service:update" }

type DeleteArgs struct {
	ServiceID string `json:"service_id"`
}

func (DeleteArgs) Kind() string { return "service:delete" }

type HealArgs struct {
	ServiceName string `json:"service_name"`
	ContainerID string `json:"container_id"`
	Reason      string `json:"reason"`
}

func (HealArgs) Kind() string { return "service:heal" }

type CleanupArgs struct {
	ServiceName string   `json:"service_name"`
	Images      []string `json:"images,omitempty"`
	Volumes     []string `json:"volumes,omitempty"`
}

func (CleanupArgs) Kind() string { return "service:cleanup" }

type HealthCheckArgs struct{}

func (HealthCheckArgs) Kind() string { return "service:healthcheck" }

type AutoscaleArgs struct{}

func (AutoscaleArgs) Kind() string { return "service:autoscale" }

// ── Shared handler ────────────────────────────────────────────────────────────

type ServiceWorkers struct {
	db           *gorm.DB
	rt           runtime.Runtime
	ingress      ingress.Adapter
	log          serviceLogger
	agentClient  *agent.Client
	notifSvc     *notification.Service
	notifier     *notification.Notifier
	scaleHistory *scaleTracker
	bus          *eventbus.Bus
}

type serviceLogger interface {
	Info(string, string, ...any)
	Warn(string, string, ...any)
	Warnw(string, string, ...any)
	Error(string, string, error, ...any)
	ContainerEvent(string, string, string, string, string)
}

func newServiceWorkers(
	db *gorm.DB,
	rt runtime.Runtime,
	ing ingress.Adapter,
	log serviceLogger,
	notifSvc *notification.Service,
	notifier *notification.Notifier,
	agentClient *agent.Client,
	bus *eventbus.Bus,
) *ServiceWorkers {
	return &ServiceWorkers{
		db:           db,
		rt:           rt,
		ingress:      ing,
		log:          log,
		agentClient:  agentClient,
		notifSvc:     notifSvc,
		notifier:     notifier,
		scaleHistory: &scaleTracker{m: make(map[string]scaleEntry)},
		bus:          bus,
	}
}

func (h *ServiceWorkers) markFailed(svc *models.Service, err error) {
	h.db.Model(svc).Updates(map[string]any{
		"status":     models.ServiceStatusFailed,
		"last_error": err.Error(),
	})
	if h.bus != nil {
		h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventDeployFailed,
			Topic: eventbus.TopicDeploy,
			Payload: eventbus.DeployFailedPayload{
				DeployID:  svc.ID.String(),
				ServiceID: svc.ID.String(),
				Error:     err.Error(),
			},
		})
	}
}

func (h *ServiceWorkers) removeContainers(ctx context.Context, serviceName string) {
	containers, err := h.rt.ListContainers(ctx, true)
	if err != nil {
		return
	}
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == serviceName || ct.Name == serviceName ||
			strings.HasPrefix(ct.Name, serviceName+"-") {
			_ = h.rt.StopContainer(ctx, ct.ID, runtime.StopOptions{})
			_ = h.rt.DeleteContainer(ctx, ct.ID, true)
		}
	}
}

func (h *ServiceWorkers) ensureNetwork(ctx context.Context, name string) error {
	if err := h.rt.CreateNetwork(ctx, name); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
			return nil
		}
		return err
	}
	return nil
}

func (h *ServiceWorkers) resolveProjectNetwork(projectID string) (string, error) {
	if projectID == "" {
		return "", nil
	}
	var project models.Project
	if err := h.db.First(&project, "id = ?", projectID).Error; err != nil {
		return "", fmt.Errorf("project %q not found: %w", projectID, err)
	}
	return project.NetworkName, nil
}

func (h *ServiceWorkers) primaryNetwork(ctx context.Context, projectID string) (string, error) {
	if err := h.ensureNetwork(ctx, proxyNetwork); err != nil {
		return "", err
	}
	projectNet, err := h.resolveProjectNetwork(projectID)
	if err != nil {
		return "", err
	}
	if projectNet == "" {
		return proxyNetwork, nil
	}
	if err := h.ensureNetwork(ctx, projectNet); err != nil {
		return "", err
	}
	return projectNet, nil
}

func (h *ServiceWorkers) restartService(ctx context.Context, svc *models.Service) error {
	if svc.ManifestJSON == "" {
		return fmt.Errorf("no manifest stored")
	}
	h.removeContainers(ctx, svc.Name)

	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return fmt.Errorf("unmarshal manifest: %w", err)
	}
	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}
	_ = h.ensureNetwork(ctx, proxyNetwork)

	isPodman := h.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(resolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()

	time.Sleep(2 * time.Second)
	return h.rt.CreateContainer(ctx, spec)
}

func (h *ServiceWorkers) buildImage(ctx context.Context, result *converter.Result) error {
	var out interface {
		Read([]byte) (int, error)
		Close() error
	}
	var err error
	switch {
	case result.BuildContext != nil:
		df := result.DockerfilePath
		if df == "" {
			df = "Dockerfile"
		}
		out, err = h.rt.BuildImageFromContext(ctx, result.BuildTag, df, result.BuildContext)
	case result.InlineDockerfile != "":
		out, err = h.rt.BuildImage(ctx, result.BuildTag, result.InlineDockerfile)
	default:
		return fmt.Errorf("no build context or dockerfile")
	}
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	buf := make([]byte, 65536)
	for {
		n, readErr := out.Read(buf)
		if n > 0 {
			var line struct {
				Error string `json:"error"`
			}
			if jsonErr := json.Unmarshal(buf[:n], &line); jsonErr == nil && line.Error != "" {
				return fmt.Errorf("build error: %s", line.Error)
			}
		}
		if readErr != nil {
			break
		}
	}
	return nil
}

func resolvedToDeployCmd(svc *models.Service, resolved *manifest.ResolvedManifest) *agentpb.CmdDeploy {
	env := make([]string, 0, len(resolved.Container.Env))
	for _, e := range resolved.Container.Env {
		env = append(env, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}
	ports := make([]*agentpb.PortSpec, 0)
	if resolved.Expose.Port > 0 {
		ports = append(ports, &agentpb.PortSpec{
			ContainerPort: int32(resolved.Expose.Port),
			HostPort:      0,
			Protocol:      "tcp",
		})
	}
	return &agentpb.CmdDeploy{
		ProjectId:   svc.ProjectID,
		ServiceName: svc.Name,
		Image:       resolved.Container.Image,
		Env:         env,
		Ports:       ports,
		Labels: map[string]string{
			"tidefly.service":    svc.Name,
			"tidefly.service-id": svc.ID.String(),
			"tidefly.project":    svc.ProjectID,
		},
		Network: proxyNetwork,
	}
}

// ── Worker Implementations ────────────────────────────────────────────────────

type DeployWorker struct {
	river.WorkerDefaults[DeployArgs]
	h *ServiceWorkers
}

func (w *DeployWorker) Work(ctx context.Context, job *river.Job[DeployArgs]) error {
	p := job.Args
	w.h.log.Info("jobs", fmt.Sprintf("service deploy started: id=%s", p.ServiceID))

	var svc models.Service
	if err := w.h.db.First(&svc, "id = ?", p.ServiceID).Error; err != nil {
		return fmt.Errorf("service %q not found: %w", p.ServiceID, err)
	}

	conv := converter.New()
	result, err := conv.Convert(ctx, converter.ConvertInput{
		ManifestJSON: p.ManifestJSON,
		Image:        p.Image,
		ComposeYAML:  p.ComposeYAML,
		Dockerfile:   p.Dockerfile,
		GitURL:       p.GitURL,
		Name:         p.Name,
		StackName:    p.StackName,
		ProjectID:    p.ProjectID,
		Domain:       p.Domain,
		Port:         p.Port,
		Expose:       p.Expose,
		Branch:       p.Branch,
		GitToken:     p.GitToken,
		Replicas:     p.Replicas,
		Strategy:     p.Strategy,
	})
	if err != nil {
		w.h.markFailed(&svc, err)
		return fmt.Errorf("convert: %w", err)
	}
	raw := result.Manifests[0]

	resolved, err := manifest.Resolve(raw)
	if err != nil {
		w.h.markFailed(&svc, err)
		return fmt.Errorf("resolve manifest: %w", err)
	}

	if resolved.Name != "" && svc.Name != resolved.Name {
		w.h.db.Model(&svc).Update("name", resolved.Name)
		svc.Name = resolved.Name
	}

	primaryNet, err := w.h.primaryNetwork(ctx, svc.ProjectID)
	if err != nil {
		w.h.markFailed(&svc, err)
		return err
	}

	if result.BuildRequired {
		w.h.log.Info("jobs", fmt.Sprintf("building image %q for service %q", result.BuildTag, svc.Name))
		if err := w.h.buildImage(ctx, result); err != nil {
			w.h.markFailed(&svc, err)
			return fmt.Errorf("build image: %w", err)
		}
	}

	for _, v := range resolved.Container.Volumes {
		if err := w.h.rt.CreateVolume(ctx, v.Name); err != nil {
			w.h.markFailed(&svc, err)
			return fmt.Errorf("create volume %q: %w", v.Name, err)
		}
	}

	isPodman := w.h.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(resolved, primaryNet, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	spec.Labels["tidefly.service"] = svc.Name
	spec.Labels["tidefly.project"] = svc.ProjectID

	if err := w.h.rt.CreateContainer(ctx, spec); err != nil {
		w.h.markFailed(&svc, err)
		return fmt.Errorf("create container: %w", err)
	}

	if raw.Spec.Expose != nil && primaryNet != proxyNetwork {
		if err := w.h.rt.ConnectNetwork(ctx, svc.Name, proxyNetwork); err != nil {
			w.h.log.Warnw("jobs", "failed to connect container to proxy network", "service", svc.Name, "error", err)
		}
	}

	var publicURL string
	if resolved.Expose.Domain != "" {
		route := caddyingress.RouteFromManifest(resolved.Name, resolved.Expose.Domain, resolved.Expose.Port, resolved.Expose.TLS, resolved.Expose.WWW)
		if err := w.h.ingress.AddRoute(ctx, route); err != nil {
			w.h.log.Warnw("jobs", "failed to register ingress route", "service", resolved.Name, "error", err)
		} else {
			scheme := "https"
			if !resolved.Expose.TLS {
				scheme = "http"
			}
			publicURL = fmt.Sprintf("%s://%s", scheme, resolved.Expose.Domain)
		}
	}

	rawJSON, _ := json.Marshal(raw)
	w.h.db.Model(&models.Service{}).Where("id = ?", svc.ID).Updates(map[string]any{
		"name":          svc.Name,
		"status":        models.ServiceStatusRunning,
		"manifest_json": string(rawJSON),
		"public_url":    publicURL,
	})

	if w.h.notifier != nil {
		var settings models.SystemSettings
		if err := w.h.db.WithContext(ctx).First(&settings).Error; err == nil &&
			settings.ExternalNotificationsEnabled && settings.NotifyOnDeploy {
			w.h.notifier.Send(ctx, notification.Event{
				Title:   fmt.Sprintf("[Deploy] %s", svc.Name),
				Message: fmt.Sprintf("Service %q successfully deployed (url: %s)", svc.Name, publicURL),
				Level:   "info",
			})
		}
	}

	if w.h.bus != nil {
		w.h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventDeployDone,
			Topic: eventbus.TopicDeploy,
			Payload: eventbus.DeployDonePayload{
				DeployID:  p.ServiceID,
				ServiceID: p.ServiceID,
			},
		})
		w.h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventServiceCreated,
			Topic: eventbus.TopicServices,
			Payload: eventbus.ServicePayload{
				ID:        svc.ID.String(),
				Name:      svc.Name,
				ProjectID: svc.ProjectID,
			},
		})
	}

	w.h.log.Info("jobs", fmt.Sprintf("service deploy complete: id=%s name=%s", p.ServiceID, svc.Name))
	return nil
}

// ── Redeploy ──────────────────────────────────────────────────────────────────

type RedeployWorker struct {
	river.WorkerDefaults[RedeployArgs]
	h *ServiceWorkers
}

func (w *RedeployWorker) Work(ctx context.Context, job *river.Job[RedeployArgs]) error {
	p := job.Args
	var svc models.Service
	if err := w.h.db.First(&svc, "id = ?", p.ServiceID).Error; err != nil {
		return fmt.Errorf("service not found: %w", err)
	}
	if svc.ManifestJSON == "" {
		return fmt.Errorf("no manifest stored for service %q", svc.Name)
	}
	w.h.db.Model(&svc).Update("status", models.ServiceStatusDeploying)

	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		w.h.markFailed(&svc, err)
		return fmt.Errorf("unmarshal manifest: %w", err)
	}
	if p.ImageOverride != "" {
		raw.Spec.Container.Image = p.ImageOverride
		raw.Spec.Container.Build = nil
	}

	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		w.h.markFailed(&svc, err)
		return fmt.Errorf("resolve: %w", err)
	}

	if resolved.Build != nil {
		var buildResult *converter.Result
		if resolved.Build.IsGit {
			tarBuf, err := converter.BuildGitContext(resolved.Build.GitURL, resolved.Build.Branch, p.GitToken)
			if err != nil {
				w.h.markFailed(&svc, err)
				return fmt.Errorf("git clone: %w", err)
			}
			buildResult = &converter.Result{
				BuildRequired:  true,
				BuildTag:       resolved.Build.Tag,
				BuildContext:   tarBuf,
				DockerfilePath: resolved.Build.DockerfilePath,
			}
		} else {
			buildResult = &converter.Result{
				BuildRequired:    true,
				BuildTag:         resolved.Build.Tag,
				InlineDockerfile: resolved.Build.DockerfileInline,
			}
		}
		if err := w.h.buildImage(ctx, buildResult); err != nil {
			w.h.markFailed(&svc, err)
			return fmt.Errorf("rebuild image: %w", err)
		}
	}

	primaryNet, _ := w.h.primaryNetwork(ctx, svc.ProjectID)

	switch resolved.Scaling.Strategy {
	case "blue-green":
		if err := deployBlueGreen(ctx, w.h, &svc, resolved); err != nil {
			w.h.markFailed(&svc, err)
			return err
		}
	default:
		w.h.removeContainers(ctx, svc.Name)
		isPodman := w.h.rt.Type() == runtime.RuntimePodman
		spec := manifest.ToContainerSpec(resolved, primaryNet, isPodman)
		spec.Labels["tidefly.service-id"] = svc.ID.String()
		spec.Labels["tidefly.service"] = svc.Name
		spec.Labels["tidefly.project"] = svc.ProjectID
		if err := w.h.rt.CreateContainer(ctx, spec); err != nil {
			w.h.markFailed(&svc, err)
			return fmt.Errorf("create container: %w", err)
		}
		if raw.Spec.Expose != nil && primaryNet != proxyNetwork {
			if err := w.h.rt.ConnectNetwork(ctx, svc.Name, proxyNetwork); err != nil {
				w.h.log.Warnw("jobs", "failed to connect to proxy network", "service", svc.Name, "error", err)
			}
		}
	}

	updatedJSON, _ := json.Marshal(&raw)
	w.h.db.Model(&svc).Updates(map[string]any{
		"status":        models.ServiceStatusRunning,
		"manifest_json": string(updatedJSON),
	})

	if w.h.bus != nil {
		w.h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventDeployDone,
			Topic: eventbus.TopicDeploy,
			Payload: eventbus.DeployDonePayload{
				DeployID:  p.ServiceID,
				ServiceID: p.ServiceID,
			},
		})
		w.h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventServiceUpdated,
			Topic: eventbus.TopicServices,
			Payload: eventbus.ServicePayload{
				ID:   svc.ID.String(),
				Name: svc.Name,
			},
		})
	}

	w.h.log.Info("jobs", fmt.Sprintf("service redeploy complete: %s (strategy=%s)", svc.Name, resolved.Scaling.Strategy))
	return nil
}

// ── Update ────────────────────────────────────────────────────────────────────

type UpdateWorker struct {
	river.WorkerDefaults[UpdateArgs]
	h *ServiceWorkers
}

func (w *UpdateWorker) Work(ctx context.Context, job *river.Job[UpdateArgs]) error {
	p := job.Args
	var svc models.Service
	if err := w.h.db.First(&svc, "id = ?", p.ServiceID).Error; err != nil {
		return fmt.Errorf("service not found: %w", err)
	}
	if svc.ManifestJSON == "" {
		return fmt.Errorf("no manifest for service %q", svc.Name)
	}

	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return fmt.Errorf("unmarshal manifest: %w", err)
	}

	changed := false
	if p.Image != "" && p.Image != raw.Spec.Container.Image {
		raw.Spec.Container.Image = p.Image
		raw.Spec.Container.Build = nil
		changed = true
	}
	if p.Domain != "" {
		if raw.Spec.Expose == nil {
			raw.Spec.Expose = &manifest.ExposeSpec{}
		}
		if raw.Spec.Expose.Domain != p.Domain {
			raw.Spec.Expose.Domain = p.Domain
			changed = true
		}
	}
	if p.Replicas > 0 {
		if raw.Spec.Scaling == nil {
			raw.Spec.Scaling = &manifest.ScalingSpec{}
		}
		if raw.Spec.Scaling.Replicas != p.Replicas {
			raw.Spec.Scaling.Replicas = p.Replicas
			changed = true
		}
	}
	if !changed {
		return nil
	}

	if p.Domain != "" {
		resolved, _ := manifest.Resolve(&raw)
		if resolved != nil && resolved.Expose.Domain != "" {
			_ = w.h.ingress.UpdateRoute(ctx, caddyingress.RouteFromManifest(
				svc.Name, resolved.Expose.Domain, resolved.Expose.Port, resolved.Expose.TLS, resolved.Expose.WWW,
			))
		}
	}

	if p.Image != "" {
		w.h.removeContainers(ctx, svc.Name)
		primaryNet, _ := w.h.primaryNetwork(ctx, svc.ProjectID)
		resolved, err := manifest.Resolve(&raw)
		if err != nil {
			return fmt.Errorf("resolve: %w", err)
		}
		isPodman := w.h.rt.Type() == runtime.RuntimePodman
		spec := manifest.ToContainerSpec(resolved, primaryNet, isPodman)
		spec.Labels["tidefly.service-id"] = svc.ID.String()
		spec.Labels["tidefly.service"] = svc.Name
		spec.Labels["tidefly.project"] = svc.ProjectID
		if err := w.h.rt.CreateContainer(ctx, spec); err != nil {
			return fmt.Errorf("create container: %w", err)
		}
		if raw.Spec.Expose != nil && primaryNet != proxyNetwork {
			if err := w.h.rt.ConnectNetwork(ctx, svc.Name, proxyNetwork); err != nil {
				w.h.log.Warnw("jobs", "proxy network connect failed", "service", svc.Name, "error", err)
			}
		}
	}

	updatedJSON, _ := json.Marshal(&raw)
	w.h.db.Model(&svc).Updates(map[string]any{
		"status":        models.ServiceStatusRunning,
		"manifest_json": string(updatedJSON),
	})

	if w.h.bus != nil {
		w.h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventServiceUpdated,
			Topic: eventbus.TopicServices,
			Payload: eventbus.ServicePayload{
				ID:   svc.ID.String(),
				Name: svc.Name,
			},
		})
	}

	return nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type DeleteWorker struct {
	river.WorkerDefaults[DeleteArgs]
	h *ServiceWorkers
}

func (w *DeleteWorker) Work(ctx context.Context, job *river.Job[DeleteArgs]) error {
	var svc models.Service
	if err := w.h.db.First(&svc, "id = ?", job.Args.ServiceID).Error; err != nil {
		return fmt.Errorf("service not found: %w", err)
	}

	var images, volumes []string
	if containers, err := w.h.rt.ListContainers(ctx, true); err == nil {
		for _, ct := range containers {
			if ct.Labels["tidefly.service"] != svc.Name {
				continue
			}
			if ct.Image != "" {
				images = append(images, ct.Image)
			}
			if details, err := w.h.rt.GetContainer(ctx, ct.ID); err == nil {
				for _, m := range details.Mounts {
					if m.Source != "" {
						volumes = append(volumes, m.Source)
					}
				}
			}
		}
	}

	_ = w.h.ingress.RemoveRoute(ctx, svc.Name)
	w.h.removeContainers(ctx, svc.Name)
	if err := w.h.db.Delete(&svc).Error; err != nil {
		return err
	}

	if w.h.bus != nil {
		w.h.bus.Publish(eventbus.Event{
			Type:  eventbus.EventServiceDeleted,
			Topic: eventbus.TopicServices,
			Payload: eventbus.ServicePayload{
				ID:        svc.ID.String(),
				Name:      svc.Name,
				ProjectID: svc.ProjectID,
			},
		})
	}

	if len(images) > 0 || len(volumes) > 0 {
		w.h.log.Info("jobs", fmt.Sprintf("delete: skipping cleanup enqueue for %q (no client in worker)", svc.Name))
	}
	return nil
}

// ── Cleanup ───────────────────────────────────────────────────────────────────

type CleanupWorker struct {
	river.WorkerDefaults[CleanupArgs]
	h *ServiceWorkers
}

func (w *CleanupWorker) Work(ctx context.Context, job *river.Job[CleanupArgs]) error {
	p := job.Args
	allContainers, err := w.h.rt.ListContainers(ctx, true)
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
		if details, err := w.h.rt.GetContainer(ctx, ct.ID); err == nil {
			for _, m := range details.Mounts {
				if m.Source != "" {
					usedVolumes[m.Source] = struct{}{}
				}
			}
		}
	}
	for _, vol := range p.Volumes {
		if _, inUse := usedVolumes[vol]; inUse {
			continue
		}
		if err := w.h.rt.DeleteVolume(ctx, vol); err != nil {
			if strings.Contains(err.Error(), "volume is in use") || strings.Contains(err.Error(), "409") {
				continue
			}
			w.h.log.Warn("jobs", fmt.Sprintf("cleanup: failed to delete volume %q: %v", vol, err))
		}
	}
	for _, img := range p.Images {
		if _, inUse := usedImages[img]; inUse {
			continue
		}
		if err := w.h.rt.DeleteImage(ctx, img, false); err != nil {
			w.h.log.Warn("jobs", fmt.Sprintf("cleanup: failed to delete image %q: %v", img, err))
		}
	}
	w.h.log.Info("jobs", fmt.Sprintf("cleanup complete: service=%s", p.ServiceName))
	return nil
}

// ── Heal ──────────────────────────────────────────────────────────────────────

type HealWorker struct {
	river.WorkerDefaults[HealArgs]
	h *ServiceWorkers
}

func (w *HealWorker) Work(ctx context.Context, job *river.Job[HealArgs]) error {
	p := job.Args
	var svc models.Service
	if err := w.h.db.Where("name = ? AND manifest_service = ?", p.ServiceName, true).First(&svc).Error; err != nil {
		return nil
	}
	w.h.log.Info("jobs", fmt.Sprintf("self-heal: triggered for %q (reason=%s)", p.ServiceName, p.Reason))

	if svc.ManifestJSON == "" ||
		svc.Status == models.ServiceStatusDeploying ||
		svc.Status == models.ServiceStatusStopped ||
		svc.Status == models.ServiceStatusRestarting {
		w.h.log.Info("jobs", fmt.Sprintf("self-heal: skipping %q (status=%s)", p.ServiceName, svc.Status))
		return nil
	}

	time.Sleep(2 * time.Second)

	if svc.WorkerID != "" && w.h.agentClient != nil && w.h.agentClient.IsConnected(svc.WorkerID) {
		var raw manifest.ServiceManifest
		if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
			return fmt.Errorf("unmarshal manifest: %w", err)
		}
		resolved, err := manifest.Resolve(&raw)
		if err != nil {
			return fmt.Errorf("resolve: %w", err)
		}
		deployCmd := resolvedToDeployCmd(&svc, resolved)
		if err := w.h.agentClient.SendHeal(ctx, svc.WorkerID, svc.Name, p.Reason, deployCmd); err != nil {
			w.h.db.Model(&svc).Update("status", models.ServiceStatusFailed)
			w.h.notifyHealFailed(ctx, svc.Name, err)
			return fmt.Errorf("worker self-heal %q: %w", p.ServiceName, err)
		}
		w.h.db.Model(&svc).Update("status", models.ServiceStatusRunning)
		w.h.notifyHealRecovered(ctx, svc.Name)
		return nil
	}

	if containers, err := w.h.rt.ListContainers(ctx, true); err == nil {
		for _, ct := range containers {
			if ct.Labels["tidefly.service"] == p.ServiceName && !runtime.NeedsRestart(ct.Status) {
				w.h.log.Info("jobs", fmt.Sprintf("self-heal: %q already recovered", p.ServiceName))
				return nil
			}
		}
	}

	if err := w.h.restartService(ctx, &svc); err != nil {
		w.h.db.Model(&svc).Update("status", models.ServiceStatusFailed)
		w.h.notifyHealFailed(ctx, svc.Name, err)
		return fmt.Errorf("self-heal %q: %w", p.ServiceName, err)
	}
	w.h.db.Model(&svc).Update("status", models.ServiceStatusRunning)
	w.h.notifyHealRecovered(ctx, svc.Name)
	return nil
}

func (h *ServiceWorkers) notifyHealFailed(ctx context.Context, serviceName string, err error) {
	if h.notifSvc == nil {
		return
	}
	msg := fmt.Sprintf("self-heal FAILED for %q: %s — manual intervention required", serviceName, err.Error())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.notifSvc.Upsert(ctx, "heal:failed:"+serviceName, serviceName, models.SeverityError, msg)
		if h.notifier != nil {
			h.notifier.Send(ctx, notification.Event{Title: fmt.Sprintf("[ERROR] %s is down", serviceName), Message: msg, Level: "error"})
		}
	}()
}

func (h *ServiceWorkers) notifyHealRecovered(ctx context.Context, serviceName string) {
	if h.notifSvc == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.notifSvc.Upsert(ctx, "heal:recovered:"+serviceName, serviceName, models.SeverityInfo,
			fmt.Sprintf("service %q recovered automatically", serviceName))
	}()
}

// ── Health Check ──────────────────────────────────────────────────────────────

type HealthCheckWorker struct {
	river.WorkerDefaults[HealthCheckArgs]
	h *ServiceWorkers
}

func (w *HealthCheckWorker) Work(ctx context.Context, _ *river.Job[HealthCheckArgs]) error {
	var services []models.Service
	if err := w.h.db.Where("manifest_service = ? AND status NOT IN ?", true, []models.ServiceStatus{
		models.ServiceStatusStopped,
		models.ServiceStatusRestarting,
	}).Find(&services).Error; err != nil {
		return fmt.Errorf("list manifest: %w", err)
	}
	if len(services) == 0 {
		return nil
	}

	containers, err := w.h.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	running := make(map[string]runtime.ContainerStatus)
	for _, ct := range containers {
		if name := ct.Labels["tidefly.service"]; name != "" {
			running[name] = ct.Status
		}
	}

	healed := 0
	for i := range services {
		svc := &services[i]

		if svc.Name == "" || svc.ManifestJSON == "" {
			if _, hasContainer := running[svc.Name]; !hasContainer {
				w.h.db.Delete(svc)
			}
			continue
		}
		if svc.Status == models.ServiceStatusDeploying {
			if _, hasContainer := running[svc.Name]; !hasContainer && time.Since(svc.UpdatedAt) > 10*time.Minute {
				w.h.db.Delete(svc)
			}
			continue
		}
		if svc.Status == models.ServiceStatusRestarting {
			continue
		}
		if svc.WorkerID != "" {
			if w.h.agentClient != nil && !w.h.agentClient.IsConnected(svc.WorkerID) {
				w.h.log.Warn("jobs", fmt.Sprintf("worker %s offline for service %q", svc.WorkerID, svc.Name))
			}
			continue
		}
		status, exists := running[svc.Name]
		if exists && !runtime.NeedsRestart(status) {
			if svc.Status != models.ServiceStatusRunning {
				w.h.db.Model(svc).Update("status", models.ServiceStatusRunning)
			}
			continue
		}
		if err := w.h.restartService(ctx, svc); err != nil {
			w.h.db.Model(svc).Update("status", models.ServiceStatusFailed)
			w.h.notifyHealFailed(ctx, svc.Name, err)
			continue
		}
		w.h.db.Model(svc).Update("status", models.ServiceStatusRunning)
		w.h.notifyHealRecovered(ctx, svc.Name)
		healed++
	}
	if healed > 0 {
		w.h.log.Info("jobs", fmt.Sprintf("healthcheck: %d service(s) restarted", healed))
	}
	return nil
}

// ── Autoscale ─────────────────────────────────────────────────────────────────

const (
	scaleUpCooldown   = 30 * time.Second
	scaleDownCooldown = 3 * time.Minute
)

type scaleEntry struct {
	lastScaleUp         time.Time
	lastScaleDown       time.Time
	belowThresholdSince time.Time
}

type scaleTracker struct {
	mu sync.Mutex
	m  map[string]scaleEntry
}

func (t *scaleTracker) get(name string) scaleEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.m[name]
}

func (t *scaleTracker) set(name string, e scaleEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[name] = e
}

type AutoscaleWorker struct {
	river.WorkerDefaults[AutoscaleArgs]
	h *ServiceWorkers
}

func (w *AutoscaleWorker) Work(ctx context.Context, _ *river.Job[AutoscaleArgs]) error {
	var services []models.Service
	if err := w.h.db.Where("manifest_service = ? AND status = ?", true, models.ServiceStatusRunning).
		Find(&services).Error; err != nil {
		return fmt.Errorf("autoscale: list manifest: %w", err)
	}
	for i := range services {
		if err := w.processAutoscale(ctx, &services[i]); err != nil {
			w.h.log.Warn("jobs", fmt.Sprintf("autoscale failed for %q: %v", services[i].Name, err))
		}
	}
	return nil
}

func (w *AutoscaleWorker) processAutoscale(ctx context.Context, svc *models.Service) error {
	if svc.ManifestJSON == "" {
		return nil
	}
	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return nil
	}
	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		return nil
	}
	as := resolved.Scaling.Autoscaling
	if !as.Enabled {
		return nil
	}

	now := time.Now()
	entry := w.h.scaleHistory.get(svc.Name)

	if svc.WorkerID != "" && w.h.agentClient != nil && w.h.agentClient.IsConnected(svc.WorkerID) {
		return w.processWorkerAutoscale(ctx, svc, resolved, as, now, entry)
	}

	containers, err := w.h.rt.ListContainers(ctx, false)
	if err != nil {
		return err
	}
	var svcContainers []runtime.Container
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] == svc.Name {
			svcContainers = append(svcContainers, ct)
		}
	}
	if len(svcContainers) == 0 {
		return nil
	}

	var totalCPU, totalMem float64
	var measured int
	for _, ct := range svcContainers {
		m, err := readContainerStats(ctx, w.h.rt, ct.ID)
		if err != nil {
			continue
		}
		totalCPU += m.cpuPercent
		totalMem += m.memPercent
		measured++
	}
	if measured == 0 {
		return nil
	}

	avgCPU := totalCPU / float64(measured)
	avgMem := totalMem / float64(measured)
	target := float64(as.Target)
	current := len(svcContainers)

	switch {
	case (avgCPU >= target || avgMem >= target) && current < as.Max:
		if now.Sub(entry.lastScaleUp) < scaleUpCooldown {
			return nil
		}
		if err := w.scaleUp(ctx, svc, resolved, current); err != nil {
			return err
		}
		entry.lastScaleUp = now
		entry.belowThresholdSince = time.Time{}
		w.h.scaleHistory.set(svc.Name, entry)

	case avgCPU < target*0.5 && avgMem < target*0.5 && current > as.Min:
		if entry.belowThresholdSince.IsZero() {
			entry.belowThresholdSince = now
			w.h.scaleHistory.set(svc.Name, entry)
			return nil
		}
		if now.Sub(entry.belowThresholdSince) < scaleDownCooldown {
			return nil
		}
		if err := w.scaleDown(ctx, svcContainers, svc.Name, current); err != nil {
			return err
		}
		entry.lastScaleDown = now
		entry.belowThresholdSince = time.Time{}
		w.h.scaleHistory.set(svc.Name, entry)

	default:
		if !entry.belowThresholdSince.IsZero() && avgCPU >= target*0.5 {
			entry.belowThresholdSince = time.Time{}
			w.h.scaleHistory.set(svc.Name, entry)
		}
	}
	return nil
}

func (w *AutoscaleWorker) scaleUp(ctx context.Context, svc *models.Service, resolved *manifest.ResolvedManifest, current int) error {
	newResolved := *resolved
	newResolved.Name = fmt.Sprintf("%s-%d", svc.Name, current+1)
	isPodman := w.h.rt.Type() == runtime.RuntimePodman
	spec := manifest.ToContainerSpec(&newResolved, proxyNetwork, isPodman)
	spec.Labels["tidefly.service-id"] = svc.ID.String()
	w.h.log.Info("jobs", fmt.Sprintf("autoscale UP: %s %d→%d", svc.Name, current, current+1))
	return w.h.rt.CreateContainer(ctx, spec)
}

func (w *AutoscaleWorker) scaleDown(ctx context.Context, containers []runtime.Container, serviceName string, current int) error {
	var toRemove *runtime.Container
	for i := range containers {
		ct := &containers[i]
		if ct.Name == serviceName {
			continue
		}
		if toRemove == nil || ct.Name > toRemove.Name {
			toRemove = ct
		}
	}
	if toRemove == nil {
		return nil
	}
	w.h.log.Info("jobs", fmt.Sprintf("autoscale DOWN: %s %d→%d (removing %s)", serviceName, current, current-1, toRemove.Name))
	_ = w.h.rt.StopContainer(ctx, toRemove.ID, runtime.StopOptions{})
	return w.h.rt.DeleteContainer(ctx, toRemove.ID, true)
}

func (w *AutoscaleWorker) processWorkerAutoscale(
	ctx context.Context,
	svc *models.Service,
	resolved *manifest.ResolvedManifest,
	as manifest.ResolvedAutoscaling,
	now time.Time,
	entry scaleEntry,
) error {
	workerContainers, err := w.h.agentClient.ListContainers(ctx, svc.WorkerID)
	if err != nil {
		return fmt.Errorf("list worker containers: %w", err)
	}
	var current int32
	for _, ct := range workerContainers {
		if ct.Labels["tidefly.service"] == svc.Name {
			current++
		}
	}
	if current == 0 {
		return nil
	}
	metrics, err := w.h.agentClient.CollectMetrics(ctx, svc.WorkerID)
	if err != nil {
		return fmt.Errorf("collect worker metrics: %w", err)
	}
	avgCPU := metrics.CpuPercent
	avgMem := metrics.MemUsedMb / metrics.MemTotalMb * 100
	target := float64(as.Target)
	deployCmd := resolvedToDeployCmd(svc, resolved)

	switch {
	case (avgCPU >= target || avgMem >= target) && current < int32(as.Max):
		if now.Sub(entry.lastScaleUp) < scaleUpCooldown {
			return nil
		}
		w.h.log.Info("jobs", fmt.Sprintf("worker autoscale UP: %s %d→%d", svc.Name, current, current+1))
		if err := w.h.agentClient.SendAutoscale(ctx, svc.WorkerID, svc.Name, current, current+1, deployCmd); err != nil {
			return err
		}
		entry.lastScaleUp = now
		entry.belowThresholdSince = time.Time{}
		w.h.scaleHistory.set(svc.Name, entry)

	case avgCPU < target*0.5 && avgMem < target*0.5 && current > int32(as.Min):
		if entry.belowThresholdSince.IsZero() {
			entry.belowThresholdSince = now
			w.h.scaleHistory.set(svc.Name, entry)
			return nil
		}
		if now.Sub(entry.belowThresholdSince) < scaleDownCooldown {
			return nil
		}
		w.h.log.Info("jobs", fmt.Sprintf("worker autoscale DOWN: %s %d→%d", svc.Name, current, current-1))
		if err := w.h.agentClient.SendAutoscale(ctx, svc.WorkerID, svc.Name, current, current-1, deployCmd); err != nil {
			return err
		}
		entry.lastScaleDown = now
		entry.belowThresholdSince = time.Time{}
		w.h.scaleHistory.set(svc.Name, entry)
	}
	return nil
}
