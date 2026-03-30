package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	containerfil "github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/containers/filter"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

type Handler struct {
	runtime  runtime.Runtime
	deployer *deploy.Deployer
	log      *logger.Logger
	db       *gorm.DB
}

func New(rt runtime.Runtime, deployer *deploy.Deployer, db *gorm.DB, log *logger.Logger) *Handler {
	return &Handler{runtime: rt, deployer: deployer, log: log, db: db}
}

type ContainerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ListInput struct{}
type ListOutput struct {
	Body []runtime.Volume
}

type DeleteInput struct {
	ID string `path:"id"`
}

type ContainersInput struct {
	ID string `path:"id"`
}
type ContainersOutput struct {
	Body []ContainerRef
}

func (h *Handler) List(ctx context.Context, _ *ListInput) (*ListOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	list, err := h.runtime.ListVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}

	managed := make([]runtime.Volume, 0, len(list))
	for _, v := range list {
		if v.Labels["tidefly-plane.managed"] == "true" {
			managed = append(managed, v)
		}
	}

	// Admins see all managed volumes
	if claims.Role == string(models.RoleAdmin) {
		return &ListOutput{Body: managed}, nil
	}

	// Members see only volumes belonging to their project containers
	allowed, err := containerfil.AllowedNetworks(h.db, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}

	// Get all containers the user can access to find their volumes
	allContainers, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	allowedVolumes := make(map[string]struct{})
	for _, ct := range allContainers {
		if ct.Labels["tidefly-plane.internal"] == "true" {
			continue
		}
		if !containerfil.ContainerAllowed(ct.Networks, allowed) {
			continue
		}
		details, err := h.runtime.GetContainer(ctx, ct.ID)
		if err != nil {
			continue
		}
		for _, m := range details.Mounts {
			if m.Source != "" {
				allowedVolumes[m.Source] = struct{}{}
			}
		}
	}

	filtered := make([]runtime.Volume, 0, len(managed))
	for _, v := range managed {
		if _, ok := allowedVolumes[v.Name]; ok {
			filtered = append(filtered, v)
		}
	}

	return &ListOutput{Body: filtered}, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	err := h.runtime.DeleteVolume(ctx, input.ID)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditVolumeDelete,
			ResourceID: input.ID,
			Success:    err == nil,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("delete volume: %w", err)
	}
	return nil, nil
}

func (h *Handler) Containers(ctx context.Context, input *ContainersInput) (*ContainersOutput, error) {
	containers, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	refs := make([]ContainerRef, 0)
	for _, ct := range containers {
		details, err := h.runtime.GetContainer(ctx, ct.ID)
		if err != nil {
			continue
		}
		for _, m := range details.Mounts {
			if strings.Contains(m.Source, input.ID) {
				refs = append(refs, ContainerRef{ID: ct.ID, Name: ct.Name})
				break
			}
		}
	}
	return &ContainersOutput{Body: refs}, nil
}
