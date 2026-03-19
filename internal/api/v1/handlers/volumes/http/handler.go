package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/services/deploy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	"gorm.io/gorm"
)

type Handler struct {
	runtime  runtime.Runtime
	deployer *deploy.Deployer
	log      *logger.Logger
}

func New(rt runtime.Runtime, deployer *deploy.Deployer, db *gorm.DB, log *logger.Logger) *Handler {
	return &Handler{runtime: rt, deployer: deployer, log: log}
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
	list, err := h.runtime.ListVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	managed := make([]runtime.Volume, 0, len(list))
	for _, v := range list {
		if v.Labels["tidefly.managed"] == "true" {
			managed = append(managed, v)
		}
	}
	return &ListOutput{Body: managed}, nil
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
