package project

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
)

type listOutput struct {
	Body []models.Project
}

type createBody struct {
	Name        string `json:"name"                  minLength:"1" maxLength:"128"`
	Description string `json:"description,omitempty" maxLength:"512"`
	Color       string `json:"color,omitempty"       minLength:"4" maxLength:"9"`
}

type createInput struct {
	Body createBody
}

type createOutput struct {
	Body *models.Project
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body *models.Project
}

type updateBody struct {
	Name        string `json:"name,omitempty"        minLength:"1" maxLength:"128"`
	Description string `json:"description,omitempty" maxLength:"512"`
	Color       string `json:"color,omitempty"       minLength:"4" maxLength:"9"`
}

type updateInput struct {
	ID   string `path:"id"`
	Body updateBody
}

type updateOutput struct {
	Body *models.Project
}

type deleteInput struct {
	ID string `path:"id"`
}

type listContainersInput struct {
	ID string `path:"id"`
}

type listContainersOutput struct {
	Body []runtime.Container
}

func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	isAdmin := claims.Role == string(models.RoleAdmin)
	list, err := h.svc.ListForUser(claims.UserID, isAdmin)
	if err != nil {
		return nil, err
	}
	return &listOutput{Body: list}, nil
}

func (h *Handler) create(ctx context.Context, input *createInput) (*createOutput, error) {
	if input.Body.Color == "" {
		input.Body.Color = "#6366f1"
	}
	networkName := "tidefly_" + input.Body.Name
	if err := h.runtime.CreateNetwork(ctx, networkName); err != nil {
		h.log.Audit(ctx, _logger.AuditEntry{
			Action:  _logger.AuditProjectCreate,
			Success: false,
			Details: fmt.Sprintf("name=%s network_create_failed err=%s", input.Body.Name, err),
		})
		return nil, fmt.Errorf("create network %q: %w", networkName, err)
	}
	p := &models.Project{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Color:       input.Body.Color,
		NetworkName: networkName,
	}
	if err := h.svc.Create(p); err != nil {
		_ = h.runtime.DeleteNetwork(ctx, networkName)
		h.log.Audit(ctx, _logger.AuditEntry{
			Action:  _logger.AuditProjectCreate,
			Success: false,
			Details: fmt.Sprintf("name=%s db_create_failed err=%s", input.Body.Name, err),
		})
		return nil, fmt.Errorf("create project: %w", err)
	}
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditProjectCreate,
		ResourceID: p.ID,
		Success:    true,
		Details:    fmt.Sprintf("name=%s network=%s", p.Name, networkName),
	})
	return &createOutput{Body: p}, nil
}

func (h *Handler) get(_ context.Context, input *getInput) (*getOutput, error) {
	p, err := h.svc.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	return &getOutput{Body: &p}, nil
}

func (h *Handler) update(ctx context.Context, input *updateInput) (*updateOutput, error) {
	p, err := h.svc.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	changes, err := h.svc.Update(&p, UpdateFields{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Color:       input.Body.Color,
	})
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditProjectUpdate,
		ResourceID: p.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("name=%s changes: %v", p.Name, changes),
	})
	if err != nil {
		return nil, err
	}
	return &updateOutput{Body: &p}, nil
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	p, err := h.svc.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	if err := h.runtime.DeleteNetwork(ctx, p.NetworkName); err != nil {
		h.log.Warn("project", fmt.Sprintf("could not delete network %q: %v", p.NetworkName, err))
	}
	err = h.svc.Delete(&p)
	h.log.Audit(ctx, _logger.AuditEntry{
		Action:     _logger.AuditProjectDelete,
		ResourceID: p.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("name=%s network=%s", p.Name, p.NetworkName),
	})
	if err != nil {
		return nil, fmt.Errorf("delete project: %w", err)
	}
	return nil, nil
}

func (h *Handler) listContainers(ctx context.Context, input *listContainersInput) (*listContainersOutput, error) {
	p, err := h.svc.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	all, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	result := make([]runtime.Container, 0)
	for _, ct := range all {
		details, err := h.runtime.GetContainer(ctx, ct.ID)
		if err != nil {
			continue
		}
		for _, n := range details.Networks {
			if n == p.NetworkName {
				result = append(result, ct)
				break
			}
		}
	}
	return &listContainersOutput{Body: result}, nil
}
