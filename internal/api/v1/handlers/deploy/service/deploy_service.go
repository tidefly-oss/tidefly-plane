package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/services/template"
)

type DeployService struct {
	db       *gorm.DB
	deployer *deploy.Deployer
	loader   *template.Loader
	caddy    *caddysvc.Client
	runtime  runtime.Runtime
}

func NewDeployService(
	db *gorm.DB,
	deployer *deploy.Deployer,
	loader *template.Loader,
	caddy *caddysvc.Client,
	rt runtime.Runtime,
) *DeployService {
	return &DeployService{db: db, deployer: deployer, loader: loader, caddy: caddy, runtime: rt}
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

	// Register Caddy route + connect to proxy network if expose=true
	var publicURL string
	if req.Expose != nil && *req.Expose && s.CaddyEnabled() && len(tmpl.Containers) > 0 {
		primary := tmpl.Containers[0]
		port := 0
		if len(primary.Ports) > 0 {
			port = primary.Ports[0].Container
		}
		if port > 0 {
			// Container name from template
			containerName := req.TemplateSlug
			if req.Fields["service_name"] != "" {
				containerName = req.Fields["service_name"]
			}

			// Connect to proxy network so Caddy can reach it
			if err := s.runtime.ConnectNetwork(ctx, containerName, "tidefly_proxy"); err != nil {
				// Non-fatal — log but continue
				_ = err
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

func (s *DeployService) Destroy(ctx context.Context, id string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid service id: %w", err)
	}
	if s.CaddyEnabled() {
		var svc models.Service
		if err := s.db.First(&svc, "id = ?", parsed).Error; err == nil {
			_ = s.caddy.RemoveRoute(ctx, caddysvc.RouteID(svc.Name))
			_ = s.runtime.DisconnectNetwork(ctx, svc.Name, "tidefly_proxy")
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
