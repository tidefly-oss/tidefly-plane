package http

import (
	"context"
	"fmt"

	deploysvc "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/deploy/service"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
)

type ListServicesInput struct{}
type ListServicesOutput struct {
	Body []models.Service
}

type DeployServiceInput struct {
	Body struct {
		TemplateSlug string            `json:"template_slug" minLength:"1" maxLength:"128"`
		ProjectID    string            `json:"project_id" format:"uuid"`
		Version      string            `json:"version,omitempty" maxLength:"64"`
		Fields       map[string]string `json:"fields,omitempty"`
		Expose       *bool             `json:"expose,omitempty"`
		CustomDomain string            `json:"custom_domain,omitempty"`
	}
}
type DeployServiceOutput struct {
	Body struct {
		Service     *models.Service              `json:"service"`
		Credentials []deploy.PlaintextCredential `json:"credentials"`
		URL         string                       `json:"url,omitempty"`
	}
}

type DeleteServiceInput struct {
	ID string `path:"id"`
}

type MarkCredentialsShownInput struct {
	ID string `path:"id"`
}

func (h *Handler) ListServices(_ context.Context, _ *ListServicesInput) (*ListServicesOutput, error) {
	services, err := h.deploy.List()
	if err != nil {
		return nil, err
	}
	return &ListServicesOutput{Body: services}, nil
}

func (h *Handler) DeployService(ctx context.Context, input *DeployServiceInput) (*DeployServiceOutput, error) {
	if input.Body.Expose != nil && *input.Body.Expose && !h.deploy.TraefikEnabled() {
		return nil, huma400("Traefik integration is not enabled on this instance")
	}

	result, err := h.deploy.Deploy(
		ctx, deploysvc.DeployRequest{
			TemplateSlug: input.Body.TemplateSlug,
			ProjectID:    input.Body.ProjectID,
			Version:      input.Body.Version,
			Fields:       input.Body.Fields,
			Expose:       input.Body.Expose,
			CustomDomain: input.Body.CustomDomain,
		},
	)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditContainerDeploy,
			ResourceID: input.Body.ProjectID,
			Success:    err == nil,
			Details: fmt.Sprintf(
				"template=%s project=%s version=%s expose=%v url=%s",
				input.Body.TemplateSlug, input.Body.ProjectID,
				input.Body.Version, input.Body.Expose, result.PublicURL,
			),
		},
	)
	if err != nil {
		if err.Error() == "template not found" {
			return nil, huma404("template not found")
		}
		return nil, err
	}

	h.notifierSvc.Send(
		ctx, notifiersvc.Event{
			Title:   "Service deployed: " + input.Body.TemplateSlug,
			Message: fmt.Sprintf("template=%s project=%s", input.Body.TemplateSlug, input.Body.ProjectID),
			Level:   "info",
		},
	)

	out := &DeployServiceOutput{}
	out.Body.Service = result.Service
	out.Body.Credentials = result.Credentials
	out.Body.URL = result.PublicURL
	return out, nil
}

func (h *Handler) DeleteService(ctx context.Context, input *DeleteServiceInput) (*struct{}, error) {
	err := h.deploy.Destroy(ctx, input.ID)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditContainerDelete,
			ResourceID: input.ID,
			Success:    err == nil,
			Details:    "service destroy",
		},
	)
	if err != nil {
		if err.Error() == "invalid service id: "+err.Error() {
			return nil, huma400("invalid service id")
		}
		return nil, err
	}
	return nil, nil
}

func (h *Handler) MarkCredentialsShown(_ context.Context, input *MarkCredentialsShownInput) (*struct{}, error) {
	if err := h.credentials.MarkShown(input.ID); err != nil {
		if err.Error() == "invalid service id: "+err.Error() {
			return nil, huma400("invalid service id")
		}
		return nil, err
	}
	return nil, nil
}
