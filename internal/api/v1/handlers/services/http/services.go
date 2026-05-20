package http

import (
	"context"
	"errors"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/services/service"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/converter"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/deploy/manifest"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

// ── Service View (merged desired + runtime state) ─────────────────────────────

// ServiceView is the unified API response combining desired state (DB/manifest)
// with live runtime state. Compatible with future autoscaling/scheduler extensions.
type ServiceView struct {
	Service models.Service       `json:"service"`
	Runtime *service.RuntimeView `json:"runtime,omitempty"`
	Drift   *service.DriftView   `json:"drift,omitempty"`
}

// ── List ──────────────────────────────────────────────────────────────────────

type ListServicesInput struct{}

type ListServicesOutput struct {
	Body []ServiceView
}

func (h *Handler) ListServices(ctx context.Context, _ *ListServicesInput) (*ListServicesOutput, error) {
	services, err := h.svc.List()
	if err != nil {
		return nil, err
	}
	views := make([]ServiceView, len(services))
	for i := range services {
		rt, drift := h.svc.BuildView(ctx, &services[i])
		views[i] = ServiceView{Service: services[i], Runtime: rt, Drift: drift}
	}
	return &ListServicesOutput{Body: views}, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type GetServiceInput struct {
	ID string `path:"id" format:"uuid"`
}

type GetServiceOutput struct {
	Body ServiceView
}

func (h *Handler) GetService(ctx context.Context, input *GetServiceInput) (*GetServiceOutput, error) {
	svc, err := h.svc.Get(input.ID)
	if err != nil {
		return nil, huma404("service not found")
	}
	rt, drift := h.svc.BuildView(ctx, svc)
	return &GetServiceOutput{Body: ServiceView{Service: *svc, Runtime: rt, Drift: drift}}, nil
}

// ── Create ────────────────────────────────────────────────────────────────────

type CreateServiceInput struct {
	Body converter.APIInput
}

type CreateServiceOutput struct {
	Body struct {
		Service *models.Service `json:"service"`
		URL     string          `json:"url,omitempty"`
	}
}

func (h *Handler) CreateService(ctx context.Context, input *CreateServiceInput) (*CreateServiceOutput, error) {
	gitToken, _ := h.resolveGitToken(input.Body.GitIntegrationID)

	result, err := h.svc.Create(ctx, input.Body, gitToken)
	if err != nil {
		svcName := input.Body.ServiceName()
		h.log.Audit(ctx, applogger.AuditEntry{
			Action:     applogger.AuditContainerDeploy,
			ResourceID: svcName,
			Success:    false,
			Details:    fmt.Sprintf("service=%s error=%s", svcName, err.Error()),
		})
		if errors.Is(err, service.ErrAlreadyExists) {
			return nil, huma409("service already exists")
		}
		if errors.Is(err, service.ErrInvalidManifest) {
			return nil, huma400(err.Error())
		}
		return nil, err
	}

	h.log.Audit(ctx, applogger.AuditEntry{
		Action:     applogger.AuditContainerDeploy,
		ResourceID: input.Body.ServiceName(),
		Success:    true,
		Details:    fmt.Sprintf("service=%s url=%s", input.Body.ServiceName(), result.URL),
	})

	out := &CreateServiceOutput{}
	out.Body.Service = result.Service
	out.Body.URL = result.URL
	return out, nil
}

// ── Create From Template ──────────────────────────────────────────────────────

type CreateFromTemplateInput struct {
	Body struct {
		Slug      string            `json:"slug"       required:"true"`
		Version   string            `json:"version,omitempty"`
		Fields    map[string]string `json:"fields"     required:"true"`
		ProjectID string            `json:"project_id" format:"uuid"`
		Expose    bool              `json:"expose,omitempty"`
		Domain    string            `json:"domain,omitempty" maxLength:"253"`
	}
}

type CreateFromTemplateOutput struct {
	Body struct {
		Service     *models.Service   `json:"service"`
		URL         string            `json:"url,omitempty"`
		Credentials map[string]string `json:"credentials,omitempty"`
	}
}

func (h *Handler) CreateServiceFromTemplate(ctx context.Context, input *CreateFromTemplateInput) (*CreateFromTemplateOutput, error) {
	if h.templateLd == nil {
		return nil, huma400("template loader not available")
	}

	tmpl, err := h.templateLd.Get(input.Body.Slug)
	if err != nil {
		return nil, huma404(fmt.Sprintf("template %q not found", input.Body.Slug))
	}

	version := input.Body.Version
	if version == "" {
		version = tmpl.DefaultVersion
	}

	resolved, err := tmpl.Resolve(input.Body.Fields, version)
	if err != nil {
		return nil, huma400(fmt.Sprintf("resolve template: %s", err.Error()))
	}
	h.log.Info("template-debug", fmt.Sprintf("resolved manifest: %s", resolved.ManifestJSON))

	apiInput := converter.APIInput{
		ManifestJSON: resolved.ManifestJSON,
		ProjectID:    input.Body.ProjectID,
		Expose:       input.Body.Expose,
		Domain:       input.Body.Domain,
	}

	result, err := h.svc.Create(ctx, apiInput, "")
	if err != nil {
		h.log.Audit(ctx, applogger.AuditEntry{
			Action:     applogger.AuditContainerDeploy,
			ResourceID: input.Body.Slug,
			Success:    false,
			Details:    fmt.Sprintf("template=%s error=%s", input.Body.Slug, err.Error()),
		})
		if errors.Is(err, service.ErrAlreadyExists) {
			return nil, huma409("service already exists")
		}
		if errors.Is(err, service.ErrInvalidManifest) {
			return nil, huma400(err.Error())
		}
		return nil, err
	}

	h.log.Audit(ctx, applogger.AuditEntry{
		Action:     applogger.AuditContainerDeploy,
		ResourceID: input.Body.Slug,
		Success:    true,
		Details:    fmt.Sprintf("template=%s service=%s", input.Body.Slug, result.Service.Name),
	})

	out := &CreateFromTemplateOutput{}
	out.Body.Service = result.Service
	out.Body.URL = result.URL
	out.Body.Credentials = resolved.Credentials
	return out, nil
}

// ── Update (PATCH) ────────────────────────────────────────────────────────────

type UpdateServiceInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Image    string            `json:"image,omitempty"    maxLength:"512"`
		Env      []manifest.EnvVar `json:"env,omitempty"`
		Replicas int               `json:"replicas,omitempty" minimum:"1" maximum:"20"`
		Domain   string            `json:"domain,omitempty"   maxLength:"253"`
	}
}

type UpdateServiceOutput struct {
	Body *models.Service
}

func (h *Handler) UpdateService(ctx context.Context, input *UpdateServiceInput) (*UpdateServiceOutput, error) {
	svc, err := h.svc.Update(ctx, input.ID, service.UpdateRequest{
		Image:    input.Body.Image,
		Env:      input.Body.Env,
		Replicas: input.Body.Replicas,
		Domain:   input.Body.Domain,
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return nil, huma404("service not found")
		}
		return nil, err
	}
	return &UpdateServiceOutput{Body: svc}, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type DeleteServiceInput struct {
	ID string `path:"id" format:"uuid"`
}

func (h *Handler) DeleteService(ctx context.Context, input *DeleteServiceInput) (*struct{}, error) {
	err := h.svc.Delete(ctx, input.ID)
	h.log.Audit(ctx, applogger.AuditEntry{
		Action:     applogger.AuditContainerDelete,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    "service destroy",
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return nil, huma404("service not found")
		}
		return nil, err
	}
	return nil, nil
}

// ── Redeploy ──────────────────────────────────────────────────────────────────

type RedeployServiceInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Image string `json:"image,omitempty" maxLength:"512"`
	}
}

type RedeployServiceOutput struct {
	Body *models.Service
}

func (h *Handler) RedeployService(ctx context.Context, input *RedeployServiceInput) (*RedeployServiceOutput, error) {
	svc, err := h.svc.Redeploy(ctx, input.ID, input.Body.Image)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return nil, huma404("service not found")
		}
		return nil, err
	}
	return &RedeployServiceOutput{Body: svc}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) resolveGitToken(integrationID string) (string, error) {
	if integrationID == "" {
		return "", nil
	}
	return h.svc.ResolveGitToken(integrationID)
}
