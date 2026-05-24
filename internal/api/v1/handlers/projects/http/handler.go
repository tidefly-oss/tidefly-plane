package http

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/projects/service"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
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

type ProjectListInput struct{}
type ProjectListOutput struct {
	Body []models.Project
}

type ProjectCreateInput struct {
	Body struct {
		Name        string `json:"name"                  minLength:"1" maxLength:"128"`
		Description string `json:"description,omitempty" maxLength:"512"`
		Color       string `json:"color,omitempty"       minLength:"4" maxLength:"9"`
	}
}
type ProjectCreateOutput struct {
	Body *models.Project
}

type ProjectGetInput struct {
	ID string `path:"id"`
}
type ProjectGetOutput struct {
	Body *models.Project
}

type ProjectUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Name        string `json:"name,omitempty"        minLength:"1" maxLength:"128"`
		Description string `json:"description,omitempty" maxLength:"512"`
		Color       string `json:"color,omitempty"       minLength:"4" maxLength:"9"`
	}
}
type ProjectUpdateOutput struct {
	Body *models.Project
}

type ProjectDeleteInput struct {
	ID string `path:"id"`
}

type ProjectListContainersInput struct {
	ID string `path:"id"`
}
type ProjectListContainersOutput struct {
	Body []runtime.Container
}

func (h *Handler) List(ctx context.Context, _ *ProjectListInput) (*ProjectListOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	isAdmin := claims.Role == string(models.RoleAdmin)
	list, err := h.projects.ListForUser(claims.UserID, isAdmin)
	if err != nil {
		return nil, err
	}
	return &ProjectListOutput{Body: list}, nil
}

func (h *Handler) Create(ctx context.Context, input *ProjectCreateInput) (*ProjectCreateOutput, error) {
	if input.Body.Color == "" {
		input.Body.Color = "#6366f1"
	}
	networkName := "tidefly_" + input.Body.Name
	if err := h.runtime.CreateNetwork(ctx, networkName); err != nil {
		h.log.Audit(ctx, logger.AuditEntry{
			Action:  logger.AuditProjectCreate,
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
	if err := h.projects.Create(p); err != nil {
		_ = h.runtime.DeleteNetwork(ctx, networkName)
		h.log.Audit(ctx, logger.AuditEntry{
			Action:  logger.AuditProjectCreate,
			Success: false,
			Details: fmt.Sprintf("name=%s db_create_failed err=%s", input.Body.Name, err),
		})
		return nil, fmt.Errorf("create project: %w", err)
	}
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditProjectCreate,
		ResourceID: p.ID,
		Success:    true,
		Details:    fmt.Sprintf("name=%s network=%s", p.Name, networkName),
	})
	return &ProjectCreateOutput{Body: p}, nil
}

func (h *Handler) Get(_ context.Context, input *ProjectGetInput) (*ProjectGetOutput, error) {
	p, err := h.projects.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	return &ProjectGetOutput{Body: &p}, nil
}

func (h *Handler) Update(ctx context.Context, input *ProjectUpdateInput) (*ProjectUpdateOutput, error) {
	p, err := h.projects.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	changes, err := h.projects.Update(&p, service.UpdateFields{
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Color:       input.Body.Color,
	})
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditProjectUpdate,
		ResourceID: p.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("name=%s changes: %v", p.Name, changes),
	})
	if err != nil {
		return nil, err
	}
	return &ProjectUpdateOutput{Body: &p}, nil
}

func (h *Handler) Delete(ctx context.Context, input *ProjectDeleteInput) (*struct{}, error) {
	p, err := h.projects.GetByID(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	if err := h.runtime.DeleteNetwork(ctx, p.NetworkName); err != nil {
		h.log.Warn("projects", fmt.Sprintf("could not delete network %q: %v", p.NetworkName, err))
	}
	err = h.projects.Delete(&p)
	h.log.Audit(ctx, logger.AuditEntry{
		Action:     logger.AuditProjectDelete,
		ResourceID: p.ID,
		Success:    err == nil,
		Details:    fmt.Sprintf("name=%s network=%s", p.Name, p.NetworkName),
	})
	if err != nil {
		return nil, fmt.Errorf("delete project: %w", err)
	}
	return nil, nil
}

func (h *Handler) ListContainers(ctx context.Context, input *ProjectListContainersInput) (*ProjectListContainersOutput, error) {
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
	return &ProjectListContainersOutput{Body: result}, nil
}
