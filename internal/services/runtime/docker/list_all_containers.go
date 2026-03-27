package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"

	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

// ListAllContainers returns every container on the host, including those
// marked tidefly.internal. Used for port-conflict detection.
func (d *Runtime) ListAllContainers(ctx context.Context) ([]runtime.Container, error) {
	list, err := d.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("docker list all containers: %w", err)
	}
	result := make([]runtime.Container, 0, len(list))
	for _, c := range list {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0][1:]
		}
		result = append(
			result, runtime.Container{
				ID:    c.ID, // full ID for reliable Inspect
				Name:  name,
				Image: c.Image,
				State: c.State,
			},
		)
	}
	return result, nil
}
