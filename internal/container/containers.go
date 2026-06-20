package container

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type listInput struct {
	All bool `query:"all" doc:"Include stopped containers"`
}

type listOutput struct {
	Body []runtime.Container
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body *runtime.ContainerDetails
}

type startInput struct {
	ID string `path:"id"`
}

type startOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

type stopInput struct {
	ID string `path:"id"`
}

type stopOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

type restartInput struct {
	ID string `path:"id"`
}

type restartOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (h *Handler) list(ctx context.Context, input *listInput) (*listOutput, error) {
	list, err := h.runtime.ListContainers(ctx, input.All)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	filtered, err := h.access.FilterContainers(ctx, list)
	if err != nil {
		return nil, huma401("unauthorized")
	}
	return &listOutput{Body: filtered}, nil
}

func (h *Handler) get(ctx context.Context, input *getInput) (*getOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	return &getOutput{Body: details}, nil
}

func (h *Handler) start(ctx context.Context, input *startInput) (*startOutput, error) {
	if ct, err := h.runtime.GetContainer(ctx, input.ID); err == nil {
		if svcName := ct.Labels["tidefly.service"]; svcName != "" {
			h.db.Model(&models.Service{}).Where("name = ?", svcName).
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
	out := &startOutput{}
	out.Body.Status = "started"
	return out, nil
}

func (h *Handler) stop(ctx context.Context, input *stopInput) (*stopOutput, error) {
	if ct, err := h.runtime.GetContainer(ctx, input.ID); err == nil {
		if svcName := ct.Labels["tidefly.service"]; svcName != "" {
			h.db.Model(&models.Service{}).Where("name = ?", svcName).
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
	out := &stopOutput{}
	out.Body.Status = "stopped"
	return out, nil
}

func (h *Handler) restart(ctx context.Context, input *restartInput) (*restartOutput, error) {
	var svcName string
	if ct, err := h.runtime.GetContainer(ctx, input.ID); err == nil {
		svcName = ct.Labels["tidefly.service"]
		if svcName != "" {
			h.db.Model(&models.Service{}).Where("name = ?", svcName).
				Update("status", models.ServiceStatusRestarting)
		}
	}
	err := h.runtime.RestartContainer(ctx, input.ID, runtime.StopOptions{})
	h.log.Audit(ctx, logger.AuditEntry{
		Action: logger.AuditContainerRestart, ResourceID: input.ID, Success: err == nil,
	})
	if err != nil {
		if svcName != "" {
			h.db.Model(&models.Service{}).Where("name = ?", svcName).
				Update("status", models.ServiceStatusFailed)
		}
		return nil, fmt.Errorf("restart container: %w", err)
	}
	if svcName != "" {
		h.db.Model(&models.Service{}).Where("name = ?", svcName).
			Update("status", models.ServiceStatusRunning)
	}
	h.publishContainerEvent(ctx, input.ID, eventbus.EventContainerUpdated)
	out := &restartOutput{}
	out.Body.Status = "restarted"
	return out, nil
}

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
