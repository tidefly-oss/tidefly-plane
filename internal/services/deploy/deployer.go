package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/services/secret"
	"github.com/tidefly-oss/tidefly-plane/internal/services/template"
)

type Deployer struct {
	rt     runtime.Runtime
	db     *gorm.DB
	loader *template.Loader
}

func New(rt runtime.Runtime, db *gorm.DB, loader ...*template.Loader) *Deployer {
	d := &Deployer{rt: rt, db: db}
	if len(loader) > 0 {
		d.loader = loader[0]
	}
	return d
}

type DeployRequest struct {
	ProjectID   string
	Version     string
	Fields      map[string]string
	ExtraLabels map[string]string

	// Git + template deploy — used by webhook deploy trigger and Git deploy wizard.
	GitIntegrationID string
	RepoURL          string
	Branch           string
	TemplateSlug     string
}

type DeployResult struct {
	Service     *models.Service       `json:"service"`
	Credentials []PlaintextCredential `json:"credentials"`
}

type PlaintextCredential struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Plaintext string `json:"plaintext"`
}

func (d *Deployer) Deploy(ctx context.Context, tmpl *template.Template, req DeployRequest) (*DeployResult, error) {
	// ── 0. Validate + load project ────────────────────────────────────────────
	if req.ProjectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	var project models.Project
	if err := d.db.First(&project, "id = ?", req.ProjectID).Error; err != nil {
		return nil, fmt.Errorf("project %q not found", req.ProjectID)
	}

	serviceID := uuid.New()

	// ── 1. Build vars ─────────────────────────────────────────────────────────
	vars := map[string]string{
		"service_id": serviceID.String(),
		"version":    req.Version,
	}
	for k, v := range req.Fields {
		vars[k] = v
	}

	// ── 2. Auto-generate service_name if not provided ─────────────────────────
	if vars["service_name"] == "" {
		name, err := secret.GenerateName(tmpl.Slug)
		if err != nil {
			return nil, fmt.Errorf("generate service name: %w", err)
		}
		vars["service_name"] = name
	}

	// ── 3. Generate + hash credentials ───────────────────────────────────────
	var plaintextCreds []PlaintextCredential
	var dbCreds []models.ServiceCredential

	for _, field := range tmpl.Fields {
		if field.Type != "credential" {
			continue
		}
		plaintext, err := secret.Generate()
		if err != nil {
			return nil, fmt.Errorf("generate secret %q: %w", field.Key, err)
		}
		hash, err := secret.Hash(plaintext)
		if err != nil {
			return nil, fmt.Errorf("hash secret %q: %w", field.Key, err)
		}
		vars[field.Key] = plaintext
		plaintextCreds = append(
			plaintextCreds, PlaintextCredential{
				Key:       field.Key,
				Label:     field.Label,
				Plaintext: plaintext,
			},
		)
		dbCreds = append(
			dbCreds, models.ServiceCredential{
				ServiceID: serviceID,
				Key:       field.Key,
				Label:     field.Label,
				Hash:      hash,
			},
		)
	}

	// ── 4. Persist service record ─────────────────────────────────────────────
	svc := &models.Service{
		ID:           serviceID,
		Name:         vars["service_name"],
		TemplateSlug: tmpl.Slug,
		Version:      req.Version,
		Status:       models.ServiceStatusDeploying,
		ProjectID:    req.ProjectID,
		Credentials:  dbCreds,
	}
	if err := d.db.Create(svc).Error; err != nil {
		return nil, fmt.Errorf("save service: %w", err)
	}

	// ── 5. Create volumes ─────────────────────────────────────────────────────
	for _, ct := range tmpl.Containers {
		for _, v := range ct.Volumes {
			volName := template.Interpolate(v.Name, vars)
			if err := d.rt.CreateVolume(ctx, volName); err != nil {
				d.markFailed(svc)
				return nil, fmt.Errorf("create volume %q: %w", volName, err)
			}
		}
	}

	// ── 6. Create + start containers in project network ───────────────────────
	isPodman := d.rt.Type() == runtime.RuntimePodman
	for i, ct := range tmpl.Containers {
		spec := buildContainerSpec(ct, vars, isPodman, project.NetworkName)
		if i == 0 && len(req.ExtraLabels) > 0 {
			for k, v := range req.ExtraLabels {
				spec.Labels[k] = v
			}
		}
		if err := d.rt.CreateContainer(ctx, spec); err != nil {
			d.markFailed(svc)
			return nil, fmt.Errorf("create container %q: %w", spec.Name, err)
		}
	}

	d.db.Model(svc).Update("status", models.ServiceStatusRunning)
	svc.Status = models.ServiceStatusRunning
	return &DeployResult{Service: svc, Credentials: plaintextCreds}, nil
}

func (d *Deployer) Destroy(ctx context.Context, serviceID uuid.UUID) error {
	containers, err := d.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	// Collect images and volumes used by this service
	var imagesToRemove []string
	var volumesToRemove []string

	for _, ct := range containers {
		if ct.Labels["tidefly.service"] != serviceID.String() {
			continue
		}

		// Stop if running
		if ct.Status == runtime.StatusRunning {
			_ = d.rt.StopContainer(ctx, ct.ID, runtime.StopOptions{})
		}

		// Collect image
		if ct.Image != "" {
			imagesToRemove = append(imagesToRemove, ct.Image)
		}

		// Get details for volumes
		details, detailErr := d.rt.GetContainer(ctx, ct.ID)
		if detailErr == nil {
			for _, m := range details.Mounts {
				if m.Source != "" {
					volumesToRemove = append(volumesToRemove, m.Source)
				}
			}
		}

		// Delete container
		if err := d.rt.DeleteContainer(ctx, ct.ID, true); err != nil {
			return fmt.Errorf("delete container %q: %w", ct.Name, err)
		}
	}

	// Delete volumes
	for _, vol := range volumesToRemove {
		_ = d.rt.DeleteVolume(ctx, vol)
	}

	// Delete images
	for _, img := range imagesToRemove {
		_ = d.rt.DeleteImage(ctx, img, false)
	}

	// Delete project network if no other services use it
	// (skip for now — network is shared per project)

	// Delete DB record
	return d.db.Where("id = ?", serviceID).Delete(&models.Service{}).Error
}

func (d *Deployer) markFailed(svc *models.Service) {
	d.db.Model(svc).Update("status", models.ServiceStatusFailed)
}

func buildContainerSpec(
	ct template.TemplateContainer,
	vars map[string]string,
	isPodman bool,
	networkName string,
) runtime.ContainerSpec {
	image := ct.Image
	if isPodman && ct.ImagePodman != "" {
		image = ct.ImagePodman
	}
	image = qualifyImage(template.Interpolate(image, vars), isPodman)
	env := make([]string, 0, len(ct.Env))
	for k, v := range ct.Env {
		env = append(env, k+"="+template.Interpolate(v, vars))
	}
	ports := make([]runtime.PortBinding, 0, len(ct.Ports))
	for _, p := range ct.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		ports = append(
			ports, runtime.PortBinding{
				HostPort:      template.Interpolate(p.Host, vars),
				ContainerPort: p.Container,
				Protocol:      proto,
			},
		)
	}
	volumes := make([]runtime.VolumeMount, 0, len(ct.Volumes))
	for _, v := range ct.Volumes {
		volumes = append(
			volumes, runtime.VolumeMount{
				Name:  template.Interpolate(v.Name, vars),
				Mount: v.Mount,
			},
		)
	}
	labels := make(map[string]string)
	for k, v := range ct.Labels {
		labels[k] = template.Interpolate(v, vars)
	}
	var hc *runtime.Healthcheck
	if ct.Healthcheck != nil {
		hc = &runtime.Healthcheck{
			Test:        template.InterpolateSlice(ct.Healthcheck.Test, vars),
			Interval:    parseDuration(ct.Healthcheck.Interval),
			Timeout:     parseDuration(ct.Healthcheck.Timeout),
			Retries:     ct.Healthcheck.Retries,
			StartPeriod: parseDuration(ct.Healthcheck.StartPeriod),
		}
	}
	restart := ct.Restart
	if restart == "" {
		restart = "unless-stopped"
	}
	return runtime.ContainerSpec{
		Name:        template.Interpolate(ct.Name, vars),
		Image:       image, // bereits interpoliert + qualifiziert
		Env:         env,
		Ports:       ports,
		Volumes:     volumes,
		Labels:      labels,
		Healthcheck: hc,
		Restart:     restart,
		Command:     template.Interpolate(ct.Command, vars),
		Network:     networkName,
	}
}

func parseDuration(s string) time.Duration {
	dur, _ := time.ParseDuration(s)
	return dur
}

// qualifyImage stellt bei Podman fehlende Registry-Präfixe voran.
// Podman erlaubt keine Short-Names wie "nginx:alpine" ohne registries.conf.
// Docker löst das automatisch mit docker.io — bei Podman machen wir es explizit.
func qualifyImage(image string, isPodman bool) string {
	if !isPodman {
		return image
	}
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 {
		first := parts[0]
		if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
			return image // bereits qualifiziert: docker.io/..., ghcr.io/..., etc.
		}
	}
	return "docker.io/" + image
}
