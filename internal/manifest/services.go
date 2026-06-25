package manifest

import (
	"context"
	"errors"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
)

type serviceView struct {
	Service models.Service `json:"service"`
	Runtime *RuntimeView   `json:"runtime,omitempty"`
	Drift   *DriftView     `json:"drift,omitempty"`
}

// ── List ──────────────────────────────────────────────────────────────────────

type listOutput struct {
	Body []serviceView
}

func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	services, err := h.mgr.List()
	if err != nil {
		return nil, err
	}
	views := make([]serviceView, len(services))
	for i := range services {
		rt, drift := h.mgr.BuildView(ctx, &services[i])
		views[i] = serviceView{Service: services[i], Runtime: rt, Drift: drift}
	}
	return &listOutput{Body: views}, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type getInput struct {
	ID string `path:"id" format:"uuid"`
}

type getOutput struct {
	Body serviceView
}

func (h *Handler) get(ctx context.Context, input *getInput) (*getOutput, error) {
	svc, err := h.mgr.Get(input.ID)
	if err != nil {
		return nil, huma404("service not found")
	}
	rt, drift := h.mgr.BuildView(ctx, svc)
	return &getOutput{Body: serviceView{Service: *svc, Runtime: rt, Drift: drift}}, nil
}

// ── Create ────────────────────────────────────────────────────────────────────

type serviceCreateInput struct {
	Body DeployInput
}

type createOutput struct {
	Body struct {
		Service *models.Service `json:"service"`
		URL     string          `json:"url,omitempty"`
	}
}

func (h *Handler) create(ctx context.Context, input *serviceCreateInput) (*createOutput, error) {
	gitToken, _ := h.resolveGitToken(input.Body.GitIntegrationID)

	result, err := h.mgr.Create(ctx, input.Body, gitToken)
	if err != nil {
		h.log.Audit(ctx, applogger.AuditEntry{
			Action:     applogger.AuditContainerDeploy,
			ResourceID: input.Body.Name,
			Success:    false,
			Details:    fmt.Sprintf("service=%s error=%s", input.Body.Name, err.Error()),
		})
		if errors.Is(err, ErrAlreadyExists) {
			return nil, huma409("service already exists")
		}
		if errors.Is(err, ErrInvalidManifest) {
			return nil, huma400(err.Error())
		}
		return nil, err
	}
	h.log.Audit(ctx, applogger.AuditEntry{
		Action:     applogger.AuditContainerDeploy,
		ResourceID: input.Body.Name,
		Success:    true,
		Details:    fmt.Sprintf("service=%s url=%s", input.Body.Name, result.URL),
	})
	out := &createOutput{}
	out.Body.Service = result.Service
	out.Body.URL = result.URL
	return out, nil
}

// ── Create From Template ──────────────────────────────────────────────────────

type createFromTemplateInput struct {
	Body struct {
		Slug      string            `json:"slug"       required:"true"`
		Version   string            `json:"version,omitempty"`
		Fields    map[string]string `json:"fields"     required:"true"`
		ProjectID string            `json:"project_id" format:"uuid"`
		Expose    bool              `json:"expose,omitempty"`
		Domain    string            `json:"domain,omitempty" maxLength:"253"`
	}
}

type createFromTemplateOutput struct {
	Body struct {
		Service     *models.Service   `json:"service"`
		URL         string            `json:"url,omitempty"`
		Credentials map[string]string `json:"credentials,omitempty"`
	}
}

func (h *Handler) createFromTemplate(ctx context.Context, input *createFromTemplateInput) (*createFromTemplateOutput, error) {
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

	deployArgs := DeployInput{
		ManifestJSON: resolved.ManifestJSON,
		ProjectID:    input.Body.ProjectID,
		Expose:       input.Body.Expose,
		Domain:       input.Body.Domain,
	}
	result, err := h.mgr.Create(ctx, deployArgs, "")
	if err != nil {
		h.log.Audit(ctx, applogger.AuditEntry{
			Action:     applogger.AuditContainerDeploy,
			ResourceID: input.Body.Slug,
			Success:    false,
			Details:    fmt.Sprintf("template=%s error=%s", input.Body.Slug, err.Error()),
		})
		if errors.Is(err, ErrAlreadyExists) {
			return nil, huma409("service already exists")
		}
		if errors.Is(err, ErrInvalidManifest) {
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
	out := &createFromTemplateOutput{}
	out.Body.Service = result.Service
	out.Body.URL = result.URL
	out.Body.Credentials = resolved.Credentials
	return out, nil
}

// ── Update ────────────────────────────────────────────────────────────────────

type serviceUpdateInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Image    string   `json:"image,omitempty"    maxLength:"512"`
		Env      []EnvVar `json:"env,omitempty"`
		Replicas int      `json:"replicas,omitempty" minimum:"1" maximum:"20"`
		Domain   string   `json:"domain,omitempty"   maxLength:"253"`
	}
}

type updateOutput struct {
	Body *models.Service
}

func (h *Handler) update(ctx context.Context, input *serviceUpdateInput) (*updateOutput, error) {
	svc, err := h.mgr.Update(ctx, input.ID, UpdateRequest{
		Image:    input.Body.Image,
		Env:      input.Body.Env,
		Replicas: input.Body.Replicas,
		Domain:   input.Body.Domain,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, huma404("service not found")
		}
		return nil, err
	}
	return &updateOutput{Body: svc}, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type deleteInput struct {
	ID string `path:"id" format:"uuid"`
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	err := h.mgr.Delete(ctx, input.ID)
	h.log.Audit(ctx, applogger.AuditEntry{
		Action:     applogger.AuditContainerDelete,
		ResourceID: input.ID,
		Success:    err == nil,
		Details:    "service destroy",
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, huma404("service not found")
		}
		return nil, err
	}
	return nil, nil
}

// ── Redeploy ──────────────────────────────────────────────────────────────────

type redeployInput struct {
	ID   string `path:"id" format:"uuid"`
	Body struct {
		Image string `json:"image,omitempty" maxLength:"512"`
	}
}

type redeployOutput struct {
	Body *models.Service
}

func (h *Handler) redeploy(ctx context.Context, input *redeployInput) (*redeployOutput, error) {
	svc, err := h.mgr.Redeploy(ctx, input.ID, input.Body.Image)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, huma404("service not found")
		}
		return nil, err
	}
	return &redeployOutput{Body: svc}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) resolveGitToken(integrationID string) (string, error) {
	if integrationID == "" {
		return "", nil
	}
	return h.mgr.ResolveGitToken(integrationID)
}
