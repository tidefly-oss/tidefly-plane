package network

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"gorm.io/gorm"
)

type Handler struct {
	runtime runtime.Runtime
	log     *logger.Logger
	access  *access.Store
	bus     *eventbus.Bus
}

func NewHandler(rt runtime.Runtime, log *logger.Logger, db *gorm.DB, bus *eventbus.Bus) *Handler {
	return &Handler{runtime: rt, log: log, access: access.NewStore(db), bus: bus}
}

type networkContainerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listOutput struct{ Body []runtime.Network }
type getInput struct {
	ID string `path:"id"`
}
type getOutput struct{ Body *runtime.Network }
type deleteInput struct {
	ID string `path:"id"`
}
type containersInput struct {
	ID string `path:"id"`
}
type containersOutput struct{ Body []networkContainerRef }

func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	userID := access.UserID(ctx)
	if userID == "" {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	list, err := h.runtime.ListNetworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	managed := make([]runtime.Network, 0, len(list))
	for _, n := range list {
		if access.IsManaged(n.Labels) {
			managed = append(managed, n)
		}
	}
	if access.IsAdmin(ctx) {
		return &listOutput{Body: managed}, nil
	}
	allowed, err := h.access.AllowedNetworks(userID)
	if err != nil {
		return nil, fmt.Errorf("check access: %w", err)
	}
	filtered := make([]runtime.Network, 0, len(managed))
	for _, n := range managed {
		if _, ok := allowed[n.Name]; ok {
			filtered = append(filtered, n)
		}
	}
	return &listOutput{Body: filtered}, nil
}

func (h *Handler) get(ctx context.Context, input *getInput) (*getOutput, error) {
	n, err := h.runtime.GetNetwork(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("network not found")
	}
	return &getOutput{Body: n}, nil
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	err := h.runtime.DeleteNetwork(ctx, input.ID)
	h.log.Audit(ctx, logger.AuditEntry{Action: logger.AuditNetworkDelete, ResourceID: input.ID, Success: err == nil})
	if err != nil {
		return nil, fmt.Errorf("delete network: %w", err)
	}
	h.bus.Publish(eventbus.Event{
		Type:    eventbus.EventNetworkDeleted,
		Topic:   eventbus.TopicNetworks,
		Payload: eventbus.NetworkDeletedPayload{ID: input.ID},
	})
	return nil, nil
}

func (h *Handler) containers(ctx context.Context, input *containersInput) (*containersOutput, error) {
	network, err := h.runtime.GetNetwork(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("network not found")
	}
	cs, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	refs := make([]networkContainerRef, 0)
	for _, ct := range cs {
		details, err := h.runtime.GetContainer(ctx, ct.ID)
		if err != nil {
			continue
		}
		for _, n := range details.Networks {
			if n == network.Name {
				refs = append(refs, networkContainerRef{ID: ct.ID, Name: ct.Name})
				break
			}
		}
	}
	return &containersOutput{Body: refs}, nil
}
