package docker

import (
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	dockernetwork "github.com/docker/docker/api/types/network"
)

func mapStatus(state string) runtime.ContainerStatus {
	switch state {
	case "running":
		return runtime.StatusRunning
	case "paused":
		return runtime.StatusPaused
	case "exited":
		return runtime.StatusExited
	case "created":
		return runtime.StatusCreated
	default:
		return runtime.StatusStopped
	}
}

func mapNetwork(n dockernetwork.Summary) runtime.Network {
	subnets := make([]runtime.NetworkSubnet, 0)
	if n.IPAM.Config != nil {
		for _, cfg := range n.IPAM.Config {
			subnets = append(subnets, runtime.NetworkSubnet{Subnet: cfg.Subnet, Gateway: cfg.Gateway})
		}
	}
	return runtime.Network{
		ID:     n.ID[:12],
		Name:   n.Name,
		Driver: n.Driver,
		Scope:  n.Scope,
		IPAM:   subnets,
		Labels: n.Labels,
	}
}
