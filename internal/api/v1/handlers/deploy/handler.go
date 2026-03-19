package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/template"
	traefiksvc "github.com/tidefly-oss/tidefly-backend/internal/services/traefik"
)

type Handler struct {
	db          *gorm.DB
	deployer    *deploy.Deployer
	loader      *template.Loader
	log         *logger.Logger
	traefik     *config.TraefikConfig
	notifSvc    *notifications.Service
	notifierSvc *notifiersvc.Service
}

func New(
	db *gorm.DB, deployer *deploy.Deployer, loader *template.Loader, log *logger.Logger,
	traefik *config.TraefikConfig, notifSvc *notifications.Service, notifierSvc *notifiersvc.Service,
) *Handler {
	if traefik == nil {
		traefik = &config.TraefikConfig{}
	}
	return &Handler{
		db: db, deployer: deployer, loader: loader, log: log, traefik: traefik, notifSvc: notifSvc,
		notifierSvc: notifierSvc,
	}
}

// ── ListServices ──────────────────────────────────────────────────────────────

type ListServicesInput struct{}
type ListServicesOutput struct {
	Body []models.Service
}

func (h *Handler) ListServices(_ context.Context, _ *ListServicesInput) (*ListServicesOutput, error) {
	var services []models.Service
	if err := h.db.Find(&services).Error; err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	return &ListServicesOutput{Body: services}, nil
}

// ── DeployService ─────────────────────────────────────────────────────────────

type DeployServiceInput struct {
	Body struct {
		TemplateSlug string            `json:"template_slug" minLength:"1" maxLength:"128" doc:"Template slug"`
		ProjectID    string            `json:"project_id" format:"uuid" doc:"Project ID"`
		Version      string            `json:"version,omitempty" maxLength:"64" doc:"Template version"`
		Fields       map[string]string `json:"fields,omitempty" doc:"Template field overrides"`
		Expose       *bool             `json:"expose,omitempty" doc:"Route primary port via Traefik"`
		CustomDomain string            `json:"custom_domain,omitempty" doc:"Custom domain (requires expose=true)"`
	}
}

type DeployServiceOutput struct {
	Body struct {
		Service     *models.Service              `json:"service"`
		Credentials []deploy.PlaintextCredential `json:"credentials"`
		URL         string                       `json:"url,omitempty"`
	}
}

func (h *Handler) DeployService(ctx context.Context, input *DeployServiceInput) (*DeployServiceOutput, error) {
	if input.Body.Expose != nil && *input.Body.Expose && !h.traefik.Enabled {
		return nil, huma.Error400BadRequest("Traefik integration is not enabled on this instance")
	}

	tmpl, err := h.loader.Get(input.Body.TemplateSlug)
	if err != nil {
		return nil, huma.Error404NotFound("template not found")
	}

	var traefikLabels map[string]string
	var publicURL string
	if input.Body.Expose != nil && *input.Body.Expose && len(tmpl.Containers) > 0 {
		primary := tmpl.Containers[0]
		port := 0
		if len(primary.Ports) > 0 {
			port = primary.Ports[0].Container
		}
		if port > 0 {
			traefikLabels = traefiksvc.LabelsFor(
				*h.traefik, traefiksvc.ServiceConfig{
					Name:         input.Body.TemplateSlug,
					Port:         port,
					CustomDomain: input.Body.CustomDomain,
				},
			)
			if input.Body.CustomDomain != "" {
				publicURL = "https://" + input.Body.CustomDomain
			} else {
				publicURL = "https://" + traefiksvc.Domain(*h.traefik, input.Body.TemplateSlug)
			}
		}
	}

	result, err := h.deployer.Deploy(
		ctx, tmpl, deploy.DeployRequest{
			ProjectID:   input.Body.ProjectID,
			Version:     input.Body.Version,
			Fields:      input.Body.Fields,
			ExtraLabels: traefikLabels,
		},
	)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditContainerDeploy, ResourceID: input.Body.ProjectID, Success: err == nil,
			Details: fmt.Sprintf(
				"template=%s project=%s version=%s expose=%v url=%s",
				input.Body.TemplateSlug, input.Body.ProjectID, input.Body.Version, input.Body.Expose, publicURL,
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("deploy service: %w", err)
	}

	msg := fmt.Sprintf("template=%s project=%s", input.Body.TemplateSlug, input.Body.ProjectID)
	h.notifierSvc.Send(
		ctx, notifiersvc.Event{
			Title:   "Service deployed: " + input.Body.TemplateSlug,
			Message: msg,
			Level:   "info",
		},
	)

	out := &DeployServiceOutput{}
	out.Body.Service = result.Service
	out.Body.Credentials = result.Credentials
	out.Body.URL = publicURL
	return out, nil
}

// ── DeleteService ─────────────────────────────────────────────────────────────

type DeleteServiceInput struct {
	ID string `path:"id" doc:"Service ID (UUID)"`
}

func (h *Handler) DeleteService(ctx context.Context, input *DeleteServiceInput) (*struct{}, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid service id")
	}
	err = h.deployer.Destroy(ctx, id)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditContainerDelete, ResourceID: input.ID, Success: err == nil,
			Details: "service destroy",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("destroy service: %w", err)
	}

	return nil, nil
}

// ── MarkCredentialsShown ──────────────────────────────────────────────────────

type MarkCredentialsShownInput struct {
	ID string `path:"id" doc:"Service ID (UUID)"`
}

func (h *Handler) MarkCredentialsShown(_ context.Context, input *MarkCredentialsShownInput) (*struct{}, error) {
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid service id")
	}
	if err := h.db.Model(&models.ServiceCredential{}).
		Where("service_id = ? AND plaintext_shown_at IS NULL", id).
		Update("plaintext_shown_at", time.Now()).Error; err != nil {
		return nil, fmt.Errorf("mark credentials shown: %w", err)
	}
	return nil, nil
}
