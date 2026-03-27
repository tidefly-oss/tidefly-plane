package http

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/projects/service"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
	"gorm.io/gorm"
)

type Handler struct {
	projects *service.ProjectService
	runtime  runtime.Runtime
	log      *logger.Logger
}

func New(db *gorm.DB, rt runtime.Runtime, log *logger.Logger) *Handler {
	return &Handler{
		projects: service.New(db),
		runtime:  rt,
		log:      log,
	}
}

type ListInput struct{}
type ListOutput struct {
	Body []models.Project
}

type CreateInput struct {
	Body struct {
		Name        string `json:"name" minLength:"1" maxLength:"128"`
		Description string `json:"description,omitempty" maxLength:"512"`
		Color       string `json:"color,omitempty" minLength:"4" maxLength:"9"`
	}
}
type CreateOutput struct {
	Body *models.Project
}

type GetInput struct {
	ID string `path:"id"`
}
type GetOutput struct {
	Body *models.Project
}

type UpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name        string `json:"name,omitempty" minLength:"1" maxLength:"128"`
		Description string `json:"description,omitempty" maxLength:"512"`
		Color       string `json:"color,omitempty" minLength:"4" maxLength:"9"`
	}
}
type UpdateOutput struct {
	Body *models.Project
}

type DeleteInput struct {
	ID string `path:"id"`
}

type ListContainersInput struct {
	ID string `path:"id"`
}
type ListContainersOutput struct {
	Body []runtime.Container
}

func (h *Handler) List(ctx context.Context, _ *ListInput) (*ListOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	isAdmin := claims.Role == string(models.RoleAdmin)
	list, err := h.projects.ListForUser(claims.UserID, isAdmin)
	if err != nil {
		return nil, err
	}
	return &ListOutput{Body: list}, nil
}
func (h *Handler) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
	if input.Body.Color == "" {
		input.Body.Color = "#6366f1"
	}
	networkName := "tidefly_" + input.Body.Name
	if err := h.runtime.CreateNetwork(ctx, networkName); err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action:  logger.AuditProjectCreate,
				Success: false,
				Details: fmt.Sprintf("name=%s network_create_failed err=%s", input.Body.Name, err),
			},
		)
		return nil, fmt.Errorf("create network %q: %w", networkName, err)
	}
	p := &models.Project{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Color:       input.Body.Color,
		NetworkName: networkName,
	}
	if err := h.projects.Create(p); err != nil {
		_ = h.runtime.DeleteNetwork(ctx, networkName)
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action:  logger.AuditProjectCreate,
				Success: false,
				Details: fmt.Sprintf("name=%s db_create_failed err=%s", input.Body.Name, err),
			},
		)
		return nil, fmt.Errorf("create project: %w", err)
	}
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditProjectCreate,
			ResourceID: p.ID,
			Success:    true,
			Details:    fmt.Sprintf("name=%s network=%s", p.Name, networkName),
		},
	)
	return &CreateOutput{Body: p}, nil
}

func (h *Handler) Get(_ context.Context, input *GetInput) (*GetOutput, error) {
	p, err := h.projects.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	return &GetOutput{Body: &p}, nil
}

func (h *Handler) Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error) {
	p, err := h.projects.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	changes, err := h.projects.Update(
		&p, service.UpdateFields{
			Name:        input.Body.Name,
			Description: input.Body.Description,
			Color:       input.Body.Color,
		},
	)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditProjectUpdate,
			ResourceID: p.ID,
			Success:    err == nil,
			Details:    fmt.Sprintf("name=%s changes: %v", p.Name, changes),
		},
	)
	if err != nil {
		return nil, err
	}
	return &UpdateOutput{Body: &p}, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	p, err := h.projects.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	if err := h.runtime.DeleteNetwork(ctx, p.NetworkName); err != nil {
		h.log.Warn("projects", fmt.Sprintf("could not delete network %q: %v", p.NetworkName, err))
	}
	err = h.projects.Delete(&p)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditProjectDelete,
			ResourceID: p.ID,
			Success:    err == nil,
			Details:    fmt.Sprintf("name=%s network=%s", p.Name, p.NetworkName),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("delete project: %w", err)
	}
	return nil, nil
}

func (h *Handler) ListContainers(ctx context.Context, input *ListContainersInput) (*ListContainersOutput, error) {
	p, err := h.projects.GetByID(input.ID)
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
	return &ListContainersOutput{Body: result}, nil
}
