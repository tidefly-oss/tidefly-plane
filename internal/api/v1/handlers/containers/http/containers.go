package http

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
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

type DeleteInput struct {
	ID    string `path:"id"`
	Force bool   `query:"force"`
}

func (h *Handler) List(ctx context.Context, input *ListInput) (*ListOutput, error) {
	list, err := h.runtime.ListContainers(ctx, input.All)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return &ListOutput{Body: list}, nil
}

func (h *Handler) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err != nil {
		return nil, huma404("container not found")
	}
	return &GetOutput{Body: details}, nil
}

func (h *Handler) Start(ctx context.Context, input *StartInput) (*StartOutput, error) {
	err := h.runtime.StartContainer(ctx, input.ID)
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditContainerStart,
			ResourceID: input.ID,
			Success:    err == nil,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}
	out := &StartOutput{}
	out.Body.Status = "started"
	return out, nil
}

func (h *Handler) Stop(ctx context.Context, input *StopInput) (*StopOutput, error) {
	err := h.runtime.StopContainer(ctx, input.ID, runtime.StopOptions{})
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditContainerStop,
			ResourceID: input.ID,
			Success:    err == nil,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("stop container: %w", err)
	}
	out := &StopOutput{}
	out.Body.Status = "stopped"
	return out, nil
}

func (h *Handler) Restart(ctx context.Context, input *RestartInput) (*RestartOutput, error) {
	err := h.runtime.RestartContainer(ctx, input.ID, runtime.StopOptions{})
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action:     logger.AuditContainerRestart,
			ResourceID: input.ID,
			Success:    err == nil,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("restart container: %w", err)
	}
	out := &RestartOutput{}
	out.Body.Status = "restarted"
	return out, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
	details, err := h.runtime.GetContainer(ctx, input.ID)
	if err == nil {
		if serviceIDStr, ok := details.Labels["tidefly.service"]; ok {
			if serviceID, parseErr := uuid.Parse(serviceIDStr); parseErr == nil {
				destroyErr := h.deployer.Destroy(ctx, serviceID)
				h.log.Audit(
					ctx, logger.AuditEntry{
						Action:     logger.AuditContainerDelete,
						ResourceID: input.ID,
						Success:    destroyErr == nil,
						Details:    fmt.Sprintf("tidefly service %s force=%v", serviceIDStr, input.Force),
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
			Action:     logger.AuditContainerDelete,
			ResourceID: input.ID,
			Success:    deleteErr == nil,
			Details:    fmt.Sprintf("force=%v", input.Force),
		},
	)
	if deleteErr != nil {
		return nil, fmt.Errorf("delete container: %w", deleteErr)
	}
	return nil, nil
}
