package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/converter"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/jobs"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

var (
	ErrNotFound        = errors.New("service not found")
	ErrAlreadyExists   = errors.New("service already exists")
	ErrInvalidManifest = errors.New("invalid manifest")
)

type CreateResult struct {
	Service *models.Service
	URL     string
}

type UpdateRequest struct {
	Image    string
	Env      []manifest.EnvVar
	Replicas int
	Domain   string
}

// RuntimeView is returned by BuildView — imported by the HTTP handler layer
// to construct ServiceView responses.
type RuntimeView struct {
	Status       string  `json:"status"`
	Replicas     int     `json:"replicas"`
	CPUPercent   float64 `json:"cpu_percent"`
	MemPercent   float64 `json:"mem_percent"`
	RestartCount int     `json:"restart_count,omitempty"`
}

// DriftView exposes reconciliation drift between desired and actual state.
type DriftView struct {
	HasDrift     bool `json:"has_drift"`
	ReplicaDrift bool `json:"replica_drift,omitempty"`
	NotRunning   bool `json:"not_running,omitempty"`
}

type ServiceManager struct {
	db       *gorm.DB
	deployer *deploy.Deployer
	queue    *asynq.Client
	log      *applogger.Logger
	gitSvc   *git.Service
	rt       runtime.Runtime
	ingress  ingress.Adapter
}

func New(
	db *gorm.DB,
	deployer *deploy.Deployer,
	queue *asynq.Client,
	log *applogger.Logger,
	gitSvc *git.Service,
	rt runtime.Runtime,
	ingressAdapter ingress.Adapter,
) *ServiceManager {
	return &ServiceManager{
		db:       db,
		deployer: deployer,
		queue:    queue,
		log:      log,
		gitSvc:   gitSvc,
		rt:       rt,
		ingress:  ingressAdapter,
	}
}

// BuildView merges the desired state from the DB with live runtime state from
// Docker/Podman. Missing runtime containers are handled gracefully — desired
// state remains queryable and drift is flagged.
func (m *ServiceManager) BuildView(ctx context.Context, svc *models.Service) (rv *RuntimeView, dv *DriftView) {
	containers, err := m.rt.ListContainers(ctx, true)
	if err != nil {
		return nil, nil
	}

	// Collect all containers belonging to this service
	var running int
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] != svc.Name {
			continue
		}
		running++
		if rv == nil {
			rv = &RuntimeView{
				Status: string(ct.Status),
			}
		}
		rv.Replicas = running
	}

	// Desired replica count from manifest
	desiredReplicas := 1
	if svc.ManifestJSON != "" {
		var raw manifest.ServiceManifest
		if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err == nil {
			if raw.Spec.Scaling != nil && raw.Spec.Scaling.Replicas > 0 {
				desiredReplicas = raw.Spec.Scaling.Replicas
			}
		}
	}

	notRunning := rv == nil || (rv.Status != string(runtime.StatusRunning))
	replicaDrift := rv != nil && rv.Replicas != desiredReplicas

	dv = &DriftView{
		NotRunning:   notRunning,
		ReplicaDrift: replicaDrift,
		HasDrift:     notRunning || replicaDrift,
	}

	return rv, dv
}

func (m *ServiceManager) List() ([]models.Service, error) {
	var services []models.Service
	if err := m.db.Where("manifest_service = ?", true).Find(&services).Error; err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	return services, nil
}

func (m *ServiceManager) Get(id string) (*models.Service, error) {
	var svc models.Service
	if err := m.db.Where("id = ? AND manifest_service = ?", id, true).First(&svc).Error; err != nil {
		return nil, ErrNotFound
	}
	return &svc, nil
}

func (m *ServiceManager) ResolveGitToken(integrationID string) (string, error) {
	if integrationID == "" {
		return "", nil
	}
	var integration models.GitIntegration
	if err := m.db.First(&integration, "id = ?", integrationID).Error; err != nil {
		return "", fmt.Errorf("git integration not found: %w", err)
	}
	return m.gitSvc.ResolveSecret(integration.SecretEncrypted)
}

func (m *ServiceManager) Create(_ context.Context, input converter.APIInput, gitToken string) (*CreateResult, error) {
	if converter.DetectType(input.ToConvertInput(gitToken)) == "" {
		return nil, fmt.Errorf("%w: provide image, compose, dockerfile, or git_url", ErrInvalidManifest)
	}
	name := input.ServiceName()
	if name == "" && input.ManifestJSON == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidManifest)
	}
	var count int64
	m.db.Model(&models.Service{}).
		Where("name = ? AND manifest_service = ?", name, true).
		Count(&count)
	if count > 0 {
		return nil, ErrAlreadyExists
	}
	svc := &models.Service{
		ID:              uuid.New(),
		Name:            name,
		Status:          models.ServiceStatusDeploying,
		ManifestService: true,
		ProjectID:       input.ProjectID,
	}
	if err := m.db.Create(svc).Error; err != nil {
		return nil, fmt.Errorf("persist service: %w", err)
	}
	if err := jobs.EnqueueServiceDeploy(m.queue, svc.ID.String(), input, gitToken); err != nil {
		m.db.Delete(svc)
		return nil, fmt.Errorf("enqueue deploy: %w", err)
	}
	return &CreateResult{Service: svc}, nil
}

//nolint:unused // reserved for upcoming multi-manifest create flow
func (m *ServiceManager) createMultiple(ctx context.Context, result *converter.Result) (*CreateResult, error) {
	var first *CreateResult
	for _, raw := range result.Manifests {
		r, err := m.Create(ctx, converter.APIInput{
			Image: raw.Spec.Container.Image,
			Name:  raw.Metadata.Name,
		}, "")
		if err != nil {
			return nil, err
		}
		if first == nil {
			first = r
		}
	}
	return first, nil
}

func (m *ServiceManager) Update(_ context.Context, id string, req UpdateRequest) (*models.Service, error) {
	svc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if err := jobs.EnqueueServiceUpdate(m.queue, id, req.Image, req.Domain, req.Replicas); err != nil {
		return nil, fmt.Errorf("enqueue update: %w", err)
	}
	svc.Status = models.ServiceStatusDeploying
	return svc, nil
}

func (m *ServiceManager) Delete(_ context.Context, id string) error {
	svc, err := m.Get(id)
	if err != nil {
		return err
	}
	m.db.Model(svc).Update("status", models.ServiceStatusStopped)
	return jobs.EnqueueServiceDelete(m.queue, id)
}

func (m *ServiceManager) Redeploy(_ context.Context, id, imageOverride string) (*models.Service, error) {
	svc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	m.db.Model(svc).Update("status", models.ServiceStatusDeploying)
	if err := jobs.EnqueueServiceRedeploy(m.queue, id, imageOverride); err != nil {
		return nil, fmt.Errorf("enqueue redeploy: %w", err)
	}
	return svc, nil
}

//nolint:unused // reserved for upcoming multi-manifest create flow
func (m *ServiceManager) portFromManifest(svc *models.Service) int {
	if svc.ManifestJSON == "" {
		return 8080
	}
	var raw manifest.ServiceManifest
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &raw); err != nil {
		return 8080
	}
	resolved, err := manifest.Resolve(&raw)
	if err != nil {
		return 8080
	}
	return resolved.Expose.Port
}
