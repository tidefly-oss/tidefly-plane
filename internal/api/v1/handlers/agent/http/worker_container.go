package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	agentpb "github.com/tidefly-oss/tidefly-plane/internal/api/v1/proto/agent"
)

type ListWorkerContainersInput struct {
	ID string `path:"id"`
}

type ContainerInfo struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Status  string            `json:"status"`
	State   string            `json:"state"`
	Created int64             `json:"created"`
	Labels  map[string]string `json:"labels"`
}

type ListWorkerContainersOutput struct {
	Body []ContainerInfo
}

func (h *Handler) ListWorkerContainers(
	ctx context.Context,
	input *ListWorkerContainersInput,
) (*ListWorkerContainersOutput, error) {
	if !h.agentClient.IsConnected(input.ID) {
		return nil, huma.Error404NotFound("worker not connected")
	}

	containers, err := h.agentClient.ListContainers(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list containers: " + err.Error())
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		result = append(result, protoToContainerInfo(c))
	}

	return &ListWorkerContainersOutput{Body: result}, nil
}

func protoToContainerInfo(c *agentpb.Container) ContainerInfo {
	return ContainerInfo{
		ID:      c.Id,
		Name:    c.Name,
		Image:   c.Image,
		Status:  c.Status,
		State:   c.State,
		Created: c.Created,
		Labels:  c.Labels,
	}
}
