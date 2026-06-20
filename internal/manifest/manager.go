package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/git"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/queue"
	"gorm.io/gorm"
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
	Env      []EnvVar
	Replicas int
	Domain   string
}

type RuntimeView struct {
	Status       string  `json:"status"`
	Replicas     int     `json:"replicas"`
	CPUPercent   float64 `json:"cpu_percent"`
	MemPercent   float64 `json:"mem_percent"`
	RestartCount int     `json:"restart_count,omitempty"`
}

type DriftView struct {
	HasDrift     bool `json:"has_drift"`
	ReplicaDrift bool `json:"replica_drift,omitempty"`
	NotRunning   bool `json:"not_running,omitempty"`
}

type Manager struct {
	db       *gorm.DB
	deployer *deploy.Deployer
	queue    *asynq.Client
	log      *applogger.Logger
	gitSvc   *git.Service
	rt       runtime.Runtime
	ingress  ingress.Adapter
}

func NewManager(
	db *gorm.DB,
	deployer *deploy.Deployer,
	q *asynq.Client,
	log *applogger.Logger,
	gitSvc *git.Service,
	rt runtime.Runtime,
	ingressAdapter ingress.Adapter,
) *Manager {
	return &Manager{
		db: db, deployer: deployer, queue: q, log: log,
		gitSvc: gitSvc, rt: rt, ingress: ingressAdapter,
	}
}

func (m *Manager) BuildView(ctx context.Context, svc *models.Service) (rv *RuntimeView, dv *DriftView) {
	containers, err := m.rt.ListContainers(ctx, true)
	if err != nil {
		return nil, nil
	}
	var running int
	for _, ct := range containers {
		if ct.Labels["tidefly.service"] != svc.Name {
			continue
		}
		running++
		if rv == nil {
			rv = &RuntimeView{Status: string(ct.Status)}
		}
		rv.Replicas = running
	}
	desiredReplicas := 1
	if svc.ManifestJSON != "" {
		var raw ServiceManifest
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

func (m *Manager) List() ([]models.Service, error) {
	var services []models.Service
	if err := m.db.Where("manifest_service = ?", true).Find(&services).Error; err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	return services, nil
}

func (m *Manager) Get(id string) (*models.Service, error) {
	var svc models.Service
	if err := m.db.Where("id = ? AND manifest_service = ?", id, true).First(&svc).Error; err != nil {
		return nil, ErrNotFound
	}
	return &svc, nil
}

func (m *Manager) ResolveGitToken(integrationID string) (string, error) {
	if integrationID == "" {
		return "", nil
	}
	var integration models.GitIntegration
	if err := m.db.First(&integration, "id = ?", integrationID).Error; err != nil {
		return "", fmt.Errorf("git integration not found: %w", err)
	}
	return m.gitSvc.ResolveSecret(integration.SecretEncrypted)
}

// Create accepts a queue.APIInput so manifest does not need to import converter.
func (m *Manager) Create(_ context.Context, input queue.APIInput, gitToken string) (*CreateResult, error) {
	name := input.ServiceName()
	if name == "" && input.ManifestJSON == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidManifest)
	}
	if input.ManifestJSON == "" && input.Image == "" && input.ComposeYAML == "" &&
		input.Dockerfile == "" && input.GitURL == "" {
		return nil, fmt.Errorf("%w: provide image, compose, dockerfile, or git_url", ErrInvalidManifest)
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
	if err := queue.EnqueueServiceDeploy(m.queue, svc.ID.String(), input, gitToken); err != nil {
		m.db.Delete(svc)
		return nil, fmt.Errorf("enqueue deploy: %w", err)
	}
	return &CreateResult{Service: svc}, nil
}

func (m *Manager) Update(_ context.Context, id string, req UpdateRequest) (*models.Service, error) {
	svc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if err := queue.EnqueueServiceUpdate(m.queue, id, req.Image, req.Domain, req.Replicas); err != nil {
		return nil, fmt.Errorf("enqueue update: %w", err)
	}
	svc.Status = models.ServiceStatusDeploying
	return svc, nil
}

func (m *Manager) Delete(_ context.Context, id string) error {
	svc, err := m.Get(id)
	if err != nil {
		return err
	}
	m.db.Model(svc).Update("status", models.ServiceStatusStopped)
	return queue.EnqueueServiceDelete(m.queue, id)
}

func (m *Manager) Redeploy(_ context.Context, id, imageOverride string) (*models.Service, error) {
	svc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	m.db.Model(svc).Update("status", models.ServiceStatusDeploying)
	if err := queue.EnqueueServiceRedeploy(m.queue, id, imageOverride); err != nil {
		return nil, fmt.Errorf("enqueue redeploy: %w", err)
	}
	return svc, nil
}
