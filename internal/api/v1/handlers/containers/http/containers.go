package http

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type ListInput struct {
	All bool `query:"all" doc:"Include stopped containers"`
}
type ListOutput struct {
	Body []runtime.Container
}
type GetInput struct {
	ID string `path:"id"`
}
type GetOutput struct {
	Body *runtime.ContainerDetails
}
type StartInput struct {
	ID string `path:"id"`
}
type StartOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}
type StopInput struct {
	ID string `path:"id"`
}
type StopOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}
type RestartInput struct {
	ID string `path:"id"`
}
type RestartOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (h *Handler) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	list, err := h.runtime.ListContainers(ctx, input.All)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	filtered, err := h.access.FilterContainers(ctx, list)
	if err != nil {
		return nil, huma401("unauthorized")
	}
	return &ListOutput{Body: filtered}, nil
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	return &GetOutput{Body: details}, nil
}

func (h *Handler) Start(ctx context.Context, input *StartInput) (*StartOutput, error) {
	// If container belongs to a service, update status to running
	if ct, err := h.runtime.GetContainer(ctx, input.ID); err == nil {
		if svcName := ct.Labels["tidefly.service"]; svcName != "" {
			h.db.Model(&models.Service{}).
				Where("name = ?", svcName).
				Update("status", models.ServiceStatusRunning)
		}
	}

	err := h.runtime.StartContainer(ctx, input.ID)
	h.log.Audit(ctx, logger.AuditEntry{
		Action: logger.AuditContainerStart, ResourceID: input.ID, Success: err == nil,
	})
	if err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	h.publishContainerEvent(ctx, input.ID, eventbus.EventContainerUpdated)
	out := &StartOutput{}
	out.Body.Status = "started"
	return out, nil
}

func (h *Handler) Stop(ctx context.Context, input *StopInput) (*StopOutput, error) {
	// Mark service as stopped before stopping — prevents self-heal from firing
	if ct, err := h.runtime.GetContainer(ctx, input.ID); err == nil {
		if svcName := ct.Labels["tidefly.service"]; svcName != "" {
			h.db.Model(&models.Service{}).
				Where("name = ?", svcName).
				Update("status", models.ServiceStatusStopped)
		}
	}

	err := h.runtime.StopContainer(ctx, input.ID, runtime.StopOptions{})
	h.log.Audit(ctx, logger.AuditEntry{
		Action: logger.AuditContainerStop, ResourceID: input.ID, Success: err == nil,
	})
	if err != nil {
		return nil, fmt.Errorf("stop container: %w", err)
	}
	h.publishContainerEvent(ctx, input.ID, eventbus.EventContainerUpdated)
	out := &StopOutput{}
	out.Body.Status = "stopped"
	return out, nil
}

func (h *Handler) Restart(ctx context.Context, input *RestartInput) (*RestartOutput, error) {
	// Mark as restarting — prevents self-heal from triggering on the kill event
	var svcName string
	if ct, err := h.runtime.GetContainer(ctx, input.ID); err == nil {
		svcName = ct.Labels["tidefly.service"]
		if svcName != "" {
			h.db.Model(&models.Service{}).
				Where("name = ?", svcName).
				Update("status", models.ServiceStatusRestarting)
		}
	}

	err := h.runtime.RestartContainer(ctx, input.ID, runtime.StopOptions{})
	h.log.Audit(ctx, logger.AuditEntry{
		Action: logger.AuditContainerRestart, ResourceID: input.ID, Success: err == nil,
	})
	if err != nil {
		// Revert status on failure
		if svcName != "" {
			h.db.Model(&models.Service{}).
				Where("name = ?", svcName).
				Update("status", models.ServiceStatusFailed)
		}
		return nil, fmt.Errorf("restart container: %w", err)
	}

	// Restore running status after successful restart
	if svcName != "" {
		h.db.Model(&models.Service{}).
			Where("name = ?", svcName).
			Update("status", models.ServiceStatusRunning)
	}

	h.publishContainerEvent(ctx, input.ID, eventbus.EventContainerUpdated)
	out := &RestartOutput{}
	out.Body.Status = "restarted"
	return out, nil
}

// publishContainerEvent fetches the current container state and broadcasts it via WS.
func (h *Handler) publishContainerEvent(ctx context.Context, containerID string, evtType string) {
	ct, err := h.runtime.GetContainer(ctx, containerID)
	if err != nil {
		return
	}
	h.bus.Publish(eventbus.Event{
		Type:  evtType,
		Topic: eventbus.TopicContainers,
		Payload: eventbus.ContainerUpdatedPayload{
			ID: ct.ID, Name: ct.Name, Status: string(ct.Status), State: ct.State,
		},
	})
}
