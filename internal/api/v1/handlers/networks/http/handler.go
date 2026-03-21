package http

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	containerfil "github.com/tidefly-oss/tidefly-backend/internal/api/v1/handlers/containers/filter"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

type Handler struct {
	runtime runtime.Runtime
	log     *logger.Logger
	db      *gorm.DB
}

func New(rt runtime.Runtime, log *logger.Logger, db *gorm.DB) *Handler {
	return &Handler{runtime: rt, log: log, db: db}
}

type NetworkContainerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ListInput struct{}
type ListOutput struct {
	Body []runtime.Network
}

type GetInput struct {
	ID string `path:"id"`
}
type GetOutput struct {
	Body *runtime.Network
}

type DeleteInput struct {
	ID string `path:"id"`
}

type ContainersInput struct {
	ID string `path:"id"`
}
type ContainersOutput struct {
	Body []NetworkContainerRef
}

func (h *Handler) List(ctx context.Context, _ *ListInput) (*ListOutput, error) {
	claims := middleware.UserFromHumaCtx(ctx)
	if claims == nil {
		return nil, huma.Error401Unauthorized("unauthorized")
	}

	list, err := h.runtime.ListNetworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}

	// Only show tidefly-managed networks
	managed := make([]runtime.Network, 0, len(list))
	for _, n := range list {
		if n.Labels["tidefly.managed"] == "true" {
			managed = append(managed, n)
		}
	}

	// Admins see all managed networks
	if claims.Role == string(models.RoleAdmin) {
		return &ListOutput{Body: managed}, nil
	}

	// Members see only networks belonging to their projects
	allowed, err := containerfil.AllowedNetworks(h.db, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}

	filtered := make([]runtime.Network, 0, len(managed))
	for _, n := range managed {
		if _, ok := allowed[n.Name]; ok {
			filtered = append(filtered, n)
		}
	}

	return &ListOutput{Body: filtered}, nil
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	network, err := h.runtime.GetNetwork(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("network not found")
	}
	return &GetOutput{Body: network}, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	err := h.runtime.DeleteNetwork(ctx, input.ID)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditNetworkDelete,
			ResourceID: input.ID,
			Success:    err == nil,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("delete network: %w", err)
	}
	return nil, nil
}

func (h *Handler) Containers(ctx context.Context, input *ContainersInput) (*ContainersOutput, error) {
	network, err := h.runtime.GetNetwork(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("network not found")
	}
	containers, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	refs := make([]NetworkContainerRef, 0)
	for _, ct := range containers {
		details, err := h.runtime.GetContainer(ctx, ct.ID)
		if err != nil {
			continue
		}
		for _, n := range details.Networks {
			if n == network.Name {
				refs = append(refs, NetworkContainerRef{ID: ct.ID, Name: ct.Name})
				break
			}
		}
	}
	return &ContainersOutput{Body: refs}, nil
}
