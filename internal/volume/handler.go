package volume

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/deploy"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"
)

type Handler struct {
	runtime  runtime.Runtime
	deployer *deploy.Deployer
	log      *_logger.Logger
	db       *gorm.DB
	bus      *_eventbus.Bus
	access   *access.Store
}

func NewHandler(rt runtime.Runtime, deployer *deploy.Deployer, db *gorm.DB, log *_logger.Logger, bus *_eventbus.Bus) *Handler {
	return &Handler{runtime: rt, deployer: deployer, log: log, db: db, bus: bus, access: access.NewStore(db)}
}

type containerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listOutput struct{ Body []runtime.Volume }
type deleteInput struct {
	ID string `path:"id"`
}
type containersInput struct {
	ID string `path:"id"`
}
type containersOutput struct{ Body []containerRef }

func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	userID := access.UserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	list, err := h.runtime.ListVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	managed := make([]runtime.Volume, 0, len(list))
	for _, v := range list {
		if access.IsManaged(v.Labels) {
			managed = append(managed, v)
		}
	}
	if access.IsAdmin(ctx) {
		return &listOutput{Body: managed}, nil
	}
	allowed, err := h.access.AllowedNetworks(userID)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}
	allContainers, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	allowedVolumes := make(map[string]struct{})
	for _, ct := range allContainers {
		if access.IsInternal(ct.Labels) {
			continue
		}
		if !access.NetworkAllowed(ct.Networks, allowed) {
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
	return &listOutput{Body: filtered}, nil
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	err := h.runtime.DeleteVolume(ctx, input.ID)
	h.log.Audit(ctx, _logger.AuditEntry{Action: _logger.AuditVolumeDelete, ResourceID: input.ID, Success: err == nil})
	if err != nil {
		return nil, fmt.Errorf("delete volume: %w", err)
	}
	h.bus.Publish(_eventbus.Event{
		Type:    _eventbus.EventVolumeDeleted,
		Topic:   _eventbus.TopicVolumes,
		Payload: _eventbus.VolumeDeletedPayload{Name: input.ID},
	})
	return nil, nil
}

func (h *Handler) containers(ctx context.Context, input *containersInput) (*containersOutput, error) {
	cs, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	refs := make([]containerRef, 0)
	for _, ct := range cs {
		details, err := h.runtime.GetContainer(ctx, ct.ID)
		if err != nil {
			continue
		}
		for _, m := range details.Mounts {
			if strings.Contains(m.Source, input.ID) {
				refs = append(refs, containerRef{ID: ct.ID, Name: ct.Name})
				break
			}
		}
	}
	return &containersOutput{Body: refs}, nil
}
