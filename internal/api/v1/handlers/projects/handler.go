package projects

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

type Handler struct {
	db      *gorm.DB
	runtime runtime.Runtime
	log     *logger.Logger
}

func New(db *gorm.DB, rt runtime.Runtime, log *logger.Logger) *Handler {
	return &Handler{db: db, runtime: rt, log: log}
}

// ── List ──────────────────────────────────────────────────────────────────────

type ListInput struct{}
type ListOutput struct {
	Body []models.Project
}

func (h *Handler) List(_ context.Context, _ *ListInput) (*ListOutput, error) {
	var list []models.Project
	if err := h.db.Order("created_at desc").Find(&list).Error; err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return &ListOutput{Body: list}, nil
}

// ── Create ────────────────────────────────────────────────────────────────────

type CreateInput struct {
	Body struct {
		Name        string `json:"name" minLength:"1" maxLength:"128" doc:"Project name"`
		Description string `json:"description,omitempty" maxLength:"512"`
		Color       string `json:"color,omitempty" minLength:"4" maxLength:"9" doc:"Hex color"`
	}
}
type CreateOutput struct {
	Body *models.Project
}

func (h *Handler) Create(ctx context.Context, input *CreateInput) (*CreateOutput, error) {
	if input.Body.Color == "" {
		input.Body.Color = "#6366f1"
	}
	networkName := "tidefly_" + input.Body.Name
	if err := h.runtime.CreateNetwork(ctx, networkName); err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditProjectCreate, Success: false,
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
	if err := h.db.Create(p).Error; err != nil {
		_ = h.runtime.DeleteNetwork(ctx, networkName)
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditProjectCreate, Success: false,
				Details: fmt.Sprintf("name=%s db_create_failed err=%s", input.Body.Name, err),
			},
		)
		return nil, fmt.Errorf("create project: %w", err)
	}
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditProjectCreate, ResourceID: p.ID, Success: true,
			Details: fmt.Sprintf("name=%s network=%s", p.Name, networkName),
		},
	)
	return &CreateOutput{Body: p}, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type GetInput struct {
	ID string `path:"id" doc:"Project ID"`
}
type GetOutput struct {
	Body *models.Project
}

func (h *Handler) Get(_ context.Context, input *GetInput) (*GetOutput, error) {
	var p models.Project
	if err := h.db.First(&p, "id = ?", input.ID).Error; err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	return &GetOutput{Body: &p}, nil
}

// ── Update ────────────────────────────────────────────────────────────────────

type UpdateInput struct {
	ID   string `path:"id" doc:"Project ID"`
	Body struct {
		Name        string `json:"name,omitempty" minLength:"1" maxLength:"128"`
		Description string `json:"description,omitempty" maxLength:"512"`
		Color       string `json:"color,omitempty" minLength:"4" maxLength:"9"`
	}
}
type UpdateOutput struct {
	Body *models.Project
}

func (h *Handler) Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error) {
	var p models.Project
	if err := h.db.First(&p, "id = ?", input.ID).Error; err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	updates := map[string]any{}
	var changes []string
	if input.Body.Name != "" && input.Body.Name != p.Name {
		updates["name"] = input.Body.Name
		changes = append(changes, fmt.Sprintf("name:%q→%q", p.Name, input.Body.Name))
	}
	if input.Body.Description != "" && input.Body.Description != p.Description {
		updates["description"] = input.Body.Description
		changes = append(changes, "description updated")
	}
	if input.Body.Color != "" && input.Body.Color != p.Color {
		updates["color"] = input.Body.Color
		changes = append(changes, fmt.Sprintf("color:%s→%s", p.Color, input.Body.Color))
	}
	if err := h.db.Model(&p).Updates(updates).Error; err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditProjectUpdate, ResourceID: p.ID, Success: false,
				Details: fmt.Sprintf("name=%s err=%s", p.Name, err),
			},
		)
		return nil, fmt.Errorf("update project: %w", err)
	}
	if len(changes) > 0 {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditProjectUpdate, ResourceID: p.ID, Success: true,
				Details: fmt.Sprintf("name=%s changes: %v", p.Name, changes),
			},
		)
	}
	return &UpdateOutput{Body: &p}, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type DeleteInput struct {
	ID string `path:"id" doc:"Project ID"`
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	var p models.Project
	if err := h.db.First(&p, "id = ?", input.ID).Error; err != nil {
		return nil, huma.Error404NotFound("project not found")
	}
	if err := h.runtime.DeleteNetwork(ctx, p.NetworkName); err != nil {
		h.log.Warn("projects", fmt.Sprintf("could not delete network %q: %v", p.NetworkName, err))
	}
	err := h.db.Delete(&p).Error
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditProjectDelete, ResourceID: p.ID, Success: err == nil,
			Details: fmt.Sprintf("name=%s network=%s", p.Name, p.NetworkName),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("delete project: %w", err)
	}
	return nil, nil
}

// ── ListContainers ────────────────────────────────────────────────────────────

type ListContainersInput struct {
	ID string `path:"id" doc:"Project ID"`
}
type ListContainersOutput struct {
	Body []runtime.Container
}

func (h *Handler) ListContainers(ctx context.Context, input *ListContainersInput) (*ListContainersOutput, error) {
	var p models.Project
	if err := h.db.First(&p, "id = ?", input.ID).Error; err != nil {
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
