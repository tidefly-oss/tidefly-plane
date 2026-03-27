package deploy

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

// Runtime returns the underlying container runtime.
// Used by jobs that need to query containers by label.
func (d *Deployer) Runtime() runtime.Runtime {
	return d.rt
}

// Redeploy stops and recreates all containers for a service using the same
// template + stored fields, but with a fresh image pull.
// It finds the service by container label tidefly.service=<serviceID>,
// then delegates to the existing Destroy + Deploy flow.
func (d *Deployer) Redeploy(ctx context.Context, serviceID string, req DeployRequest) error {
	// Find the service record in DB
	var svc models.Service
	if err := d.db.WithContext(ctx).First(&svc, "id = ?", serviceID).Error; err != nil {
		return fmt.Errorf("service %q not found: %w", serviceID, err)
	}

	// Parse UUID for Destroy
	parsedID := svc.ID

	// Destroy existing containers (stops, removes containers + images, keeps volumes)
	if err := d.Destroy(ctx, parsedID); err != nil {
		return fmt.Errorf("destroy existing service: %w", err)
	}

	// Load the template
	if d.loader == nil {
		return fmt.Errorf("template loader not configured on deployer")
	}
	tmpl, err := d.loader.Get(svc.TemplateSlug)
	if err != nil {
		return fmt.Errorf("template %q not found: %w", svc.TemplateSlug, err)
	}

	// Merge fields: stored service fields + override from webhook
	fields := make(map[string]string)
	for k, v := range req.Fields {
		fields[k] = v
	}
	if req.Version != "" {
		fields["version"] = req.Version
	}

	_, err = d.Deploy(
		ctx, tmpl, DeployRequest{
			ProjectID:   svc.ProjectID,
			Version:     req.Version,
			Fields:      fields,
			ExtraLabels: req.ExtraLabels,
		},
	)
	return err
}

// DeployFromTemplate runs a full deploy from a template slug + optional Git source.
// Returns the new service ID as a string.
func (d *Deployer) DeployFromTemplate(ctx context.Context, req DeployRequest) (string, error) {
	if req.TemplateSlug == "" {
		return "", fmt.Errorf("template_slug is required")
	}
	if req.ProjectID == "" {
		return "", fmt.Errorf("project_id is required")
	}
	if d.loader == nil {
		return "", fmt.Errorf("template loader not configured on deployer")
	}

	tmpl, err := d.loader.Get(req.TemplateSlug)
	if err != nil {
		return "", fmt.Errorf("template %q not found: %w", req.TemplateSlug, err)
	}

	// Inject Git context into fields
	fields := make(map[string]string)
	for k, v := range req.Fields {
		fields[k] = v
	}
	if req.RepoURL != "" {
		fields["GIT_REPO"] = req.RepoURL
	}
	if req.Branch != "" {
		fields["GIT_BRANCH"] = req.Branch
	}
	if req.Version != "" {
		fields["GIT_COMMIT"] = req.Version
	}

	result, err := d.Deploy(
		ctx, tmpl, DeployRequest{
			ProjectID:   req.ProjectID,
			Version:     req.Version,
			Fields:      fields,
			ExtraLabels: req.ExtraLabels,
		},
	)
	if err != nil {
		return "", err
	}

	return result.Service.ID.String(), nil
}
