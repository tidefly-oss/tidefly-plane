package docker

import (
	"context"
	"fmt"

	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

func (d *Runtime) ListNetworks(ctx context.Context) ([]runtime.Network, error) {
	list, err := d.client.NetworkList(ctx, dockernetwork.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker list networks: %w", err)
	}
	result := make([]runtime.Network, 0, len(list))
	for _, n := range list {
		if n.Labels["tidefly.internal"] == "true" || n.Labels["tidefly.managed"] != "true" {
			continue
		}
		result = append(result, mapNetwork(n))
	}
	return result, nil
}

func (d *Runtime) GetNetwork(ctx context.Context, id string) (*runtime.Network, error) {
	n, err := d.client.NetworkInspect(ctx, id, dockernetwork.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker inspect network: %w", err)
	}
	return new(mapNetwork(n)), nil
}

func (d *Runtime) CreateNetwork(ctx context.Context, name string) error {
	_, err := d.client.NetworkCreate(
		ctx, name, dockernetwork.CreateOptions{
			Driver: "bridge",
			Labels: map[string]string{"tidefly.managed": "true"},
		},
	)
	return err
}

func (d *Runtime) DeleteNetwork(ctx context.Context, id string) error {
	return d.client.NetworkRemove(ctx, id)
}

func (d *Runtime) ConnectNetwork(ctx context.Context, containerID, networkName string) error {
	err := d.client.NetworkConnect(ctx, networkName, containerID, &dockernetwork.EndpointSettings{})
	if err != nil {
		return fmt.Errorf("connect network %q to container %q: %w", networkName, containerID, err)
	}
	return nil
}

func (d *Runtime) DisconnectNetwork(ctx context.Context, containerID, networkName string) error {
	err := d.client.NetworkDisconnect(ctx, networkName, containerID, true)
	if err != nil {
		return fmt.Errorf("disconnect network %q from container %q: %w", networkName, containerID, err)
	}
	return nil
}
