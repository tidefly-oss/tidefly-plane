package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/services/agent"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/services/template"
)

type DeployService struct {
	db          *gorm.DB
	deployer    *deploy.Deployer
	loader      *template.Loader
	caddy       *caddysvc.Client
	runtime     runtime.Runtime
	agentClient *agentsvc.Client
}

func NewDeployService(
	db *gorm.DB,
	deployer *deploy.Deployer,
	loader *template.Loader,
	caddy *caddysvc.Client,
	rt runtime.Runtime,
	agentClient *agentsvc.Client,
) *DeployService {
	return &DeployService{
		db:          db,
		deployer:    deployer,
		loader:      loader,
		caddy:       caddy,
		runtime:     rt,
		agentClient: agentClient,
	}
}

func (s *DeployService) List() ([]models.Service, error) {
	var services []models.Service
	if err := s.db.Find(&services).Error; err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	return services, nil
}

type DeployRequest struct {
	TemplateSlug string
	ProjectID    string
	Version      string
	Fields       map[string]string
	Expose       *bool
	CustomDomain string
	WorkerID     string // empty = deploy locally on Plane
}

type DeployResult struct {
	Service     *models.Service
	Credentials []deploy.PlaintextCredential
	PublicURL   string
}

func (s *DeployService) Deploy(ctx context.Context, req DeployRequest) (DeployResult, error) {
	tmpl, err := s.loader.Get(req.TemplateSlug)
	if err != nil {
		return DeployResult{}, fmt.Errorf("template not found: %w", err)
	}

	expose := req.Expose != nil && *req.Expose

	// ── Worker deploy ──────────────────────────────────────────────────────────
	if req.WorkerID != "" {
		return s.deployToWorker(ctx, req, tmpl, expose)
	}

	// ── Local deploy (Plane) ───────────────────────────────────────────────────
	return s.deployLocal(ctx, req, tmpl, expose)
}

func (s *DeployService) deployLocal(
	ctx context.Context,
	req DeployRequest,
	tmpl *template.Template,
	expose bool,
) (DeployResult, error) {
	result, err := s.deployer.Deploy(
		ctx, tmpl, deploy.DeployRequest{
			ProjectID: req.ProjectID,
			Version:   req.Version,
			Fields:    req.Fields,
		},
	)
	if err != nil {
		return DeployResult{}, fmt.Errorf("deploy service: %w", err)
	}

	var publicURL string
	if expose && s.CaddyEnabled() && len(tmpl.Containers) > 0 {
		primary := tmpl.Containers[0]
		port := 0
		if len(primary.Ports) > 0 {
			port = primary.Ports[0].Container
		}
		if port > 0 {
			containerName := req.TemplateSlug
			if req.Fields["service_name"] != "" {
				containerName = req.Fields["service_name"]
			}
			if err := s.runtime.ConnectNetwork(ctx, containerName, "tidefly_proxy"); err != nil {
				_ = err // non-fatal
			}
			domain := req.CustomDomain
			if domain == "" {
				domain = caddysvc.Domain(s.caddy.Config(), containerName)
			}
			upstream := fmt.Sprintf("%s:%d", containerName, port)
			routeID := caddysvc.RouteID(containerName)
			if err := s.caddy.AddHTTPRoute(ctx, routeID, domain, upstream); err == nil {
				publicURL = "https://" + domain
			}
		}
	}

	return DeployResult{
		Service:     result.Service,
		Credentials: result.Credentials,
		PublicURL:   publicURL,
	}, nil
}

func (s *DeployService) deployToWorker(
	ctx context.Context,
	req DeployRequest,
	tmpl *template.Template,
	expose bool,
) (DeployResult, error) {
	if s.agentClient == nil {
		return DeployResult{}, fmt.Errorf("agent client not available")
	}
	if !s.agentClient.IsConnected(req.WorkerID) {
		return DeployResult{}, fmt.Errorf("worker %s is not connected", req.WorkerID)
	}

	// Build deploy request for worker
	if len(tmpl.Containers) == 0 {
		return DeployResult{}, fmt.Errorf("template has no containers")
	}

	var project models.Project
	if err := s.db.First(&project, "id = ?", req.ProjectID).Error; err != nil {
		return DeployResult{}, fmt.Errorf("project not found: %w", err)
	}

	primary := tmpl.Containers[0]

	// Resolve service name
	serviceName := req.Fields["service_name"]
	if serviceName == "" {
		serviceName = tmpl.Slug
	}

	// Build env slice
	env := make([]string, 0, len(primary.Env))
	for k, v := range primary.Env {
		env = append(env, k+"="+v)
	}

	deployResult, err := s.agentClient.Deploy(
		ctx, req.WorkerID, agentsvc.DeployRequest{
			ProjectID:   req.ProjectID,
			ServiceName: serviceName,
			Image:       primary.Image,
			Env:         env,
			Network:     project.NetworkName,
			Labels: map[string]string{
				"tidefly-plane.managed": "true",
				"tidefly-plane.project": req.ProjectID,
			},
		},
	)
	if err != nil {
		return DeployResult{}, fmt.Errorf("worker deploy: %w", err)
	}

	// Persist service record
	serviceID := uuid.New()
	svc := &models.Service{
		ID:           serviceID,
		Name:         serviceName,
		TemplateSlug: tmpl.Slug,
		Version:      req.Version,
		Status:       models.ServiceStatusRunning,
		ProjectID:    req.ProjectID,
		WorkerID:     req.WorkerID,
	}
	if err := s.db.Create(svc).Error; err != nil {
		return DeployResult{}, fmt.Errorf("save service: %w", err)
	}

	// Register Caddy route on worker if expose=true
	var publicURL string
	if expose && len(primary.Ports) > 0 {
		port := primary.Ports[0].Container
		domain := req.CustomDomain
		if domain == "" && s.CaddyEnabled() {
			domain = caddysvc.Domain(s.caddy.Config(), serviceName)
		}
		if domain != "" {
			upstream := fmt.Sprintf("%s:%d", deployResult.ContainerId, port)
			if err := s.agentClient.RegisterRoute(ctx, req.WorkerID, upstream, domain, true); err != nil {
				_ = err
			} else {
				publicURL = "https://" + domain
			}
		}
	}

	return DeployResult{
		Service:   svc,
		PublicURL: publicURL,
	}, nil
}

func (s *DeployService) Destroy(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid service id: %w", err)
	}
	if s.CaddyEnabled() {
		var svc models.Service
		if err := s.db.First(&svc, "id = ?", parsed).Error; err == nil {
			if svc.WorkerID != "" && s.agentClient != nil {
				_ = s.agentClient.RemoveRoute(ctx, svc.WorkerID, caddysvc.RouteID(svc.Name))
			} else {
				_ = s.caddy.RemoveRoute(ctx, caddysvc.RouteID(svc.Name))
				_ = s.runtime.DisconnectNetwork(ctx, svc.Name, "tidefly_proxy")
			}
		}
	}
	if err := s.deployer.Destroy(ctx, parsed); err != nil {
		return fmt.Errorf("destroy service: %w", err)
	}
	return nil
}

func (s *DeployService) CaddyEnabled() bool {
	return s.caddy != nil
}
