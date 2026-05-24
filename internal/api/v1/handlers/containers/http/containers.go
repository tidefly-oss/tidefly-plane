package http

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type ContainerListInput struct {
	All bool `query:"all" doc:"Include stopped containers"`
}
type ContainerListOutput struct {
	Body []runtime.Container
}

type ContainerGetInput struct {
	ID string `path:"id"`
}
type ContainerGetOutput struct {
	Body *runtime.ContainerDetails
}

type ContainerStartInput struct {
	ID string `path:"id"`
}
type ContainerStartOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

type ContainerStopInput struct {
	ID string `path:"id"`
}
type ContainerStopOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

type ContainerRestartInput struct {
	ID string `path:"id"`
}
type ContainerRestartOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (h *Handler) List(ctx context.Context, input *ContainerListInput) (*ContainerListOutput, error) {
	list, err := h.runtime.ListContainers(ctx, input.All)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	filtered, err := h.access.FilterContainers(ctx, list)
	if err != nil {
		return nil, huma401("unauthorized")
	}
	return &ContainerListOutput{Body: filtered}, nil
}

func (h *Handler) Get(ctx context.Context, input *ContainerGetInput) (*ContainerGetOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	return &ContainerGetOutput{Body: details}, nil
}

func (h *Handler) Start(ctx context.Context, input *ContainerStartInput) (*ContainerStartOutput, error) {
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
	out := &ContainerStartOutput{}
	out.Body.Status = "started"
	return out, nil
}

func (h *Handler) Stop(ctx context.Context, input *ContainerStopInput) (*ContainerStopOutput, error) {
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
	out := &ContainerStopOutput{}
	out.Body.Status = "stopped"
	return out, nil
}

func (h *Handler) Restart(ctx context.Context, input *ContainerRestartInput) (*ContainerRestartOutput, error) {
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
		if svcName != "" {
			h.db.Model(&models.Service{}).
				Where("name = ?", svcName).
				Update("status", models.ServiceStatusFailed)
		}
		return nil, fmt.Errorf("restart container: %w", err)
	}
	if svcName != "" {
		h.db.Model(&models.Service{}).
			Where("name = ?", svcName).
			Update("status", models.ServiceStatusRunning)
	}
	h.publishContainerEvent(ctx, input.ID, eventbus.EventContainerUpdated)
	out := &ContainerRestartOutput{}
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
