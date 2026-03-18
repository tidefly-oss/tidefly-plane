package containers

// Huma-typisierte JSON-Endpoints.
// SSE/WS-Endpoints (Logs, Stats, Exec, BuildAndDeploy) sind in exec.go / build.go.

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

type Handler struct {
	runtime  runtime.Runtime
	deployer *deploy.Deployer
	db       *gorm.DB
	log      *logger.Logger
	traefik  *config.TraefikConfig
}

func New(
	rt runtime.Runtime, deployer *deploy.Deployer, db *gorm.DB, log *logger.Logger, traefik *config.TraefikConfig,
) *Handler {
	if traefik == nil {
		traefik = &config.TraefikConfig{}
	}
	return &Handler{runtime: rt, deployer: deployer, db: db, log: log, traefik: traefik}
}

// ── List ──────────────────────────────────────────────────────────────────────

type ListInput struct {
	All bool `query:"all" doc:"Include stopped containers"`
}
type ListOutput struct {
	Body []runtime.Container
}

func (h *Handler) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	list, err := h.runtime.ListContainers(ctx, input.All)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return &ListOutput{Body: list}, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type GetInput struct {
	ID string `path:"id" doc:"Container ID"`
}
type GetOutput struct {
	Body *runtime.ContainerDetails
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("container not found")
	}
	return &GetOutput{Body: details}, nil
}

// ── Start ─────────────────────────────────────────────────────────────────────

type StartInput struct {
	ID string `path:"id" doc:"Container ID"`
}
type StartOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (h *Handler) Start(ctx context.Context, input *StartInput) (*StartOutput, error) {
	err := h.runtime.StartContainer(ctx, input.ID)
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditContainerStart, ResourceID: input.ID, Success: err == nil})
	if err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	out := &StartOutput{}
	out.Body.Status = "started"
	return out, nil
}

// ── Stop ──────────────────────────────────────────────────────────────────────

type StopInput struct {
	ID string `path:"id" doc:"Container ID"`
}
type StopOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (h *Handler) Stop(ctx context.Context, input *StopInput) (*StopOutput, error) {
	err := h.runtime.StopContainer(ctx, input.ID, runtime.StopOptions{})
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditContainerStop, ResourceID: input.ID, Success: err == nil})
	if err != nil {
		return nil, fmt.Errorf("stop container: %w", err)
	}
	out := &StopOutput{}
	out.Body.Status = "stopped"
	return out, nil
}

// ── Restart ───────────────────────────────────────────────────────────────────

type RestartInput struct {
	ID string `path:"id" doc:"Container ID"`
}
type RestartOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (h *Handler) Restart(ctx context.Context, input *RestartInput) (*RestartOutput, error) {
	err := h.runtime.RestartContainer(ctx, input.ID, runtime.StopOptions{})
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditContainerRestart, ResourceID: input.ID, Success: err == nil})
	if err != nil {
		return nil, fmt.Errorf("restart container: %w", err)
	}
	out := &RestartOutput{}
	out.Body.Status = "restarted"
	return out, nil
}

// ── Delete ────────────────────────────────────────────────────────────────────

type DeleteInput struct {
	ID    string `path:"id" doc:"Container ID"`
	Force bool   `query:"force" doc:"Force remove"`
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err == nil {
		if serviceIDStr, ok := details.Labels["tidefly.service"]; ok {
			if serviceID, parseErr := uuid.Parse(serviceIDStr); parseErr == nil {
				destroyErr := h.deployer.Destroy(ctx, serviceID)
				h.log.Audit(
					ctx, logger.AuditEntry{
						Action: logger.AuditContainerDelete, ResourceID: input.ID, Success: destroyErr == nil,
						Details: fmt.Sprintf("tidefly service %s force=%v", serviceIDStr, input.Force),
					},
				)
				if destroyErr != nil {
					return nil, fmt.Errorf("destroy service: %w", destroyErr)
				}
				return nil, nil
			}
		}
	}
	deleteErr := h.runtime.DeleteContainer(ctx, input.ID, input.Force)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditContainerDelete, ResourceID: input.ID, Success: deleteErr == nil,
			Details: fmt.Sprintf("force=%v", input.Force),
		},
	)
	if deleteErr != nil {
		return nil, fmt.Errorf("delete container: %w", deleteErr)
	}
	return nil, nil
}

// ── GetResources ──────────────────────────────────────────────────────────────

type GetResourcesInput struct {
	ID string `path:"id" doc:"Container ID"`
}
type GetResourcesOutput struct {
	Body *runtime.ResourceConfig
}

func (h *Handler) GetResources(ctx context.Context, input *GetResourcesInput) (*GetResourcesOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("container not found")
	}
	if err := checkContainerAccessHuma(ctx, h.db, details.Labels); err != nil {
		return nil, err
	}
	cfg, err := h.runtime.GetResources(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("get resources: %w", err)
	}
	return &GetResourcesOutput{Body: cfg}, nil
}

// ── UpdateResources ───────────────────────────────────────────────────────────

type UpdateResourcesInput struct {
	ID   string `path:"id" doc:"Container ID"`
	Body struct {
		CPUCores      float64 `json:"cpu_cores" minimum:"0"`
		MemoryMB      int64   `json:"memory_mb" minimum:"0"`
		MemorySwapMB  int64   `json:"memory_swap_mb" minimum:"-1"`
		RestartPolicy string  `json:"restart_policy,omitempty" enum:"no,always,on-failure,unless-stopped"`
		MaxRetries    int     `json:"max_retries" minimum:"0"`
	}
}
type UpdateResourcesOutput struct {
	Body struct {
		RestartRequired bool     `json:"restart_required"`
		Applied         []string `json:"applied"`
		Message         string   `json:"message"`
	}
}

func (h *Handler) UpdateResources(ctx context.Context, input *UpdateResourcesInput) (*UpdateResourcesOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("container not found")
	}
	if err := checkContainerAccessHuma(ctx, h.db, details.Labels); err != nil {
		return nil, err
	}
	if input.Body.MemoryMB > 0 && input.Body.MemoryMB < 6 {
		return nil, huma.Error422UnprocessableEntity("memory_mb must be >= 6 or 0 (unlimited)")
	}
	if input.Body.MemoryMB > 0 && input.Body.MemorySwapMB > 0 && input.Body.MemorySwapMB < input.Body.MemoryMB {
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf(
				"memory_swap_mb must be >= memory_mb (%d)", input.Body.MemoryMB,
			),
		)
	}
	if input.Body.MaxRetries > 0 && input.Body.RestartPolicy != "on-failure" {
		return nil, huma.Error422UnprocessableEntity("max_retries only valid with restart_policy=on-failure")
	}

	cfg := runtime.ResourceConfig{
		CPUCores: input.Body.CPUCores, MemoryMB: input.Body.MemoryMB,
		MemorySwapMB: input.Body.MemorySwapMB, RestartPolicy: input.Body.RestartPolicy,
		MaxRetries: input.Body.MaxRetries,
	}
	result, err := h.runtime.UpdateResources(ctx, input.ID, cfg)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditContainerUpdate, ResourceID: input.ID, Success: err == nil,
			Details: fmt.Sprintf(
				"cpu=%.2f mem=%dMB swap=%dMB restart=%s retries=%d",
				input.Body.CPUCores, input.Body.MemoryMB, input.Body.MemorySwapMB, input.Body.RestartPolicy,
				input.Body.MaxRetries,
			),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("update resources: %w", err)
	}
	out := &UpdateResourcesOutput{}
	out.Body.RestartRequired = result.RestartRequired
	out.Body.Applied = result.Applied
	if result.RestartRequired {
		out.Body.Message = "Memory limit reduced — container was restarted to apply changes"
	} else if len(result.Applied) == 0 {
		out.Body.Message = "No changes applied"
	} else {
		out.Body.Message = "Resource limits updated live"
	}
	return out, nil
}

// ── Access check für Huma-Context ─────────────────────────────────────────────

func checkContainerAccessHuma(ctx context.Context, db *gorm.DB, labels map[string]string) error {
	user := middleware.UserFromHumaCtx(ctx)
	if user == nil {
		return huma.Error401Unauthorized("unauthorized")
	}
	u, ok := user.(*models.User)
	if !ok || u.IsAdmin() {
		return nil
	}
	projectID, exists := labels["tidefly.project_id"]
	if !exists || projectID == "" {
		return huma.Error403Forbidden("access denied: container is not part of any project")
	}
	var count int64
	if err := db.Table("project_members").
		Where("project_id = ? AND user_id = ?", projectID, u.ID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("access check: %w", err)
	}
	if count == 0 {
		return huma.Error403Forbidden("access denied: you are not a member of this container's project")
	}
	return nil
}
