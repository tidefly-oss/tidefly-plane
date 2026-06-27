package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/git"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

var (
	ErrNotFound        = errors.New("service not found")
	ErrAlreadyExists   = errors.New("service already exists")
	ErrInvalidManifest = errors.New("invalid manifest")
)

// Lokale River JobArgs — identische JSON-Felder wie jobs.DeployArgs,
// gleiche Kind() Strings → River Worker in jobs package pickt sie auf.
// Kein Import von jobs nötig → kein Zyklus.

type deployArgs struct {
	ServiceID        string `json:"service_id"`
	ManifestJSON     string `json:"manifest_json,omitempty"`
	Image            string `json:"image,omitempty"`
	ComposeYAML      string `json:"compose_yaml,omitempty"`
	Dockerfile       string `json:"dockerfile,omitempty"`
	GitURL           string `json:"git_url,omitempty"`
	GitToken         string `json:"git_token,omitempty"`
	Name             string `json:"name,omitempty"`
	StackName        string `json:"stack_name,omitempty"`
	ProjectID        string `json:"project_id,omitempty"`
	Domain           string `json:"domain,omitempty"`
	Port             int    `json:"port,omitempty"`
	Expose           bool   `json:"expose,omitempty"`
	Branch           string `json:"branch,omitempty"`
	GitIntegrationID string `json:"git_integration_id,omitempty"`
	Replicas         int    `json:"replicas,omitempty"`
	Strategy         string `json:"strategy,omitempty"`
}

func (deployArgs) Kind() string { return "service:deploy" }

type redeployArgs struct {
	ServiceID     string `json:"service_id"`
	ImageOverride string `json:"image_override,omitempty"`
	GitToken      string `json:"git_token,omitempty"`
}

func (redeployArgs) Kind() string { return "service:redeploy" }

type updateArgs struct {
	ServiceID string `json:"service_id"`
	Image     string `json:"image,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Replicas  int    `json:"replicas,omitempty"`
}

func (updateArgs) Kind() string { return "service:update" }

type deleteArgs struct {
	ServiceID string `json:"service_id"`
}

func (deleteArgs) Kind() string { return "service:delete" }

// DeployInput ist der HTTP-seitige Input-Type (von manifest.go benutzt).
type DeployInput struct {
	ManifestJSON     string `json:"manifest_json,omitempty"`
	Image            string `json:"image,omitempty"`
	ComposeYAML      string `json:"compose,omitempty"`
	Dockerfile       string `json:"dockerfile,omitempty"`
	GitURL           string `json:"git_url,omitempty"`
	Name             string `json:"name,omitempty"`
	StackName        string `json:"stack_name,omitempty"`
	ProjectID        string `json:"project_id,omitempty"`
	Domain           string `json:"domain,omitempty"`
	Port             int    `json:"port,omitempty"`
	Expose           bool   `json:"expose,omitempty"`
	Branch           string `json:"branch,omitempty"`
	GitIntegrationID string `json:"git_integration_id,omitempty"`
	Replicas         int    `json:"replicas,omitempty"`
	Strategy         string `json:"strategy,omitempty"`
}

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
	queue    *river.Client[pgx.Tx]
	log      *applogger.Logger
	gitSvc   *git.Service
	rt       runtime.Runtime
	ingress  ingress.Adapter
}

func NewManager(
	db *gorm.DB,
	deployer *deploy.Deployer,
	q *river.Client[pgx.Tx],
	log *applogger.Logger,
	gitSvc *git.Service,
	rt runtime.Runtime,
	ingressAdapter ingress.Adapter,
) *Manager {
	return &Manager{db: db, deployer: deployer, queue: q, log: log, gitSvc: gitSvc, rt: rt, ingress: ingressAdapter}
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
	notRunning := rv == nil || rv.Status != string(runtime.StatusRunning)
	replicaDrift := rv != nil && rv.Replicas != desiredReplicas
	dv = &DriftView{NotRunning: notRunning, ReplicaDrift: replicaDrift, HasDrift: notRunning || replicaDrift}
	return rv, dv
}

func (m *Manager) List() ([]models.Service, error) {
	var services []models.Service
	if err := m.db.Where("manifest_service = ?", true).Find(&services).Error; err != nil {
		return nil, fmt.Errorf("list manifest: %w", err)
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

func (m *Manager) Create(ctx context.Context, input DeployInput, gitToken string) (*CreateResult, error) {
	name := input.Name
	if name == "" && input.ManifestJSON == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidManifest)
	}
	if input.ManifestJSON == "" && input.Image == "" && input.ComposeYAML == "" && input.Dockerfile == "" && input.GitURL == "" {
		return nil, fmt.Errorf("%w: provide image, compose, dockerfile, or git_url", ErrInvalidManifest)
	}
	var count int64
	m.db.Model(&models.Service{}).Where("name = ? AND manifest_service = ?", name, true).Count(&count)
	if count > 0 {
		return nil, ErrAlreadyExists
	}
	svc := &models.Service{
		ID: uuid.New(), Name: name,
		Status: models.ServiceStatusDeploying, ManifestService: true, ProjectID: input.ProjectID,
	}
	if err := m.db.Create(svc).Error; err != nil {
		return nil, fmt.Errorf("persist service: %w", err)
	}
	if _, err := m.queue.Insert(ctx, deployArgs{
		ServiceID: svc.ID.String(), ManifestJSON: input.ManifestJSON, Image: input.Image,
		ComposeYAML: input.ComposeYAML, Dockerfile: input.Dockerfile, GitURL: input.GitURL,
		GitToken: gitToken, Name: input.Name, StackName: input.StackName, ProjectID: input.ProjectID,
		Domain: input.Domain, Port: input.Port, Expose: input.Expose, Branch: input.Branch,
		GitIntegrationID: input.GitIntegrationID, Replicas: input.Replicas, Strategy: input.Strategy,
	}, &river.InsertOpts{Queue: "critical"}); err != nil {
		m.db.Delete(svc)
		return nil, fmt.Errorf("enqueue deploy: %w", err)
	}
	return &CreateResult{Service: svc}, nil
}

func (m *Manager) Update(ctx context.Context, id string, req UpdateRequest) (*models.Service, error) {
	svc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	if _, err := m.queue.Insert(ctx, updateArgs{ServiceID: id, Image: req.Image, Domain: req.Domain, Replicas: req.Replicas},
		&river.InsertOpts{Queue: "critical"}); err != nil {
		return nil, fmt.Errorf("enqueue update: %w", err)
	}
	svc.Status = models.ServiceStatusDeploying
	return svc, nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	svc, err := m.Get(id)
	if err != nil {
		return err
	}
	m.db.Model(svc).Update("status", models.ServiceStatusStopped)
	_, err = m.queue.Insert(ctx, deleteArgs{ServiceID: id}, &river.InsertOpts{Queue: "critical"})
	return err
}

func (m *Manager) Redeploy(ctx context.Context, id, imageOverride string) (*models.Service, error) {
	svc, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	m.db.Model(svc).Update("status", models.ServiceStatusDeploying)
	if _, err := m.queue.Insert(ctx, redeployArgs{ServiceID: id, ImageOverride: imageOverride},
		&river.InsertOpts{Queue: "critical"}); err != nil {
		return nil, fmt.Errorf("enqueue redeploy: %w", err)
	}
	return svc, nil
}
