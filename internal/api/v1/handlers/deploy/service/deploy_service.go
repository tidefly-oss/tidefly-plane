package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/template"
	traefiksvc "github.com/tidefly-oss/tidefly-backend/internal/services/traefik"
)

type DeployService struct {
	db       *gorm.DB
	deployer *deploy.Deployer
	loader   *template.Loader
	traefik  *config.TraefikConfig
}

func NewDeployService(
	db *gorm.DB,
	deployer *deploy.Deployer,
	loader *template.Loader,
	traefik *config.TraefikConfig,
) *DeployService {
	return &DeployService{db: db, deployer: deployer, loader: loader, traefik: traefik}
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

	var traefikLabels map[string]string
	var publicURL string
	if req.Expose != nil && *req.Expose && len(tmpl.Containers) > 0 {
		primary := tmpl.Containers[0]
		port := 0
		if len(primary.Ports) > 0 {
			port = primary.Ports[0].Container
		}
		if port > 0 {
			traefikLabels = traefiksvc.LabelsFor(
				*s.traefik, traefiksvc.ServiceConfig{
					Name:         req.TemplateSlug,
					Port:         port,
					CustomDomain: req.CustomDomain,
				},
			)
			if req.CustomDomain != "" {
				publicURL = "https://" + req.CustomDomain
			} else {
				publicURL = "https://" + traefiksvc.Domain(*s.traefik, req.TemplateSlug)
			}
		}
	}

	result, err := s.deployer.Deploy(
		ctx, tmpl, deploy.DeployRequest{
			ProjectID:   req.ProjectID,
			Version:     req.Version,
			Fields:      req.Fields,
			ExtraLabels: traefikLabels,
		},
	)
	if err != nil {
		return DeployResult{}, fmt.Errorf("deploy service: %w", err)
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
	if err := s.deployer.Destroy(ctx, parsed); err != nil {
		return fmt.Errorf("destroy service: %w", err)
	}
	return nil
}

func (s *DeployService) TraefikEnabled() bool {
	return s.traefik != nil && s.traefik.Enabled
}
