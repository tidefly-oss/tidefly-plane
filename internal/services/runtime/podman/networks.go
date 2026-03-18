package podman

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

func (p *Runtime) ListNetworks(ctx context.Context) ([]runtime.Network, error) {
	var raw []podmanNetwork
	code, err := p.c.getJSON(ctx, "/libpod/networks/json", nil, &raw)
	if err != nil {
		return nil, fmt.Errorf("podman list networks: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("podman list networks: status %d", code)
	}

	result := make([]runtime.Network, 0, len(raw))
	for _, n := range raw {
		if n.Labels["tidefly.internal"] == "true" || n.Labels["tidefly.managed"] != "true" {
			continue
		}
		result = append(result, mapPodmanNetwork(n))
	}
	return result, nil
}

func (p *Runtime) GetNetwork(ctx context.Context, id string) (*runtime.Network, error) {
	var raw podmanNetwork
	code, err := p.c.getJSON(ctx, "/libpod/networks/"+escPath(id)+"/json", nil, &raw)
	if err != nil {
		return nil, fmt.Errorf("podman inspect network: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("podman inspect network: status %d", code)
	}
	return new(mapPodmanNetwork(raw)), nil
}

func (p *Runtime) CreateNetwork(ctx context.Context, name string) error {
	body := map[string]any{
		"Name":       name,
		"Driver":     "bridge",
		"DNSEnabled": true,
		"Labels":     map[string]string{"tidefly.managed": "true"},
	}
	code, _, err := p.c.post(ctx, "/libpod/networks/create", nil, body)
	if err != nil {
		return fmt.Errorf("podman create network %q: %w", name, err)
	}
	if code != 200 && code != 201 {
		return fmt.Errorf("podman create network %q: status %d", name, code)
	}
	return nil
}

func (p *Runtime) DeleteNetwork(ctx context.Context, id string) error {
	code, err := p.c.delete(ctx, "/libpod/networks/"+escPath(id), nil)
	if err != nil {
		return fmt.Errorf("podman delete network %q: %w", id, err)
	}
	if code != 200 && code != 204 {
		return fmt.Errorf("podman delete network %q: status %d", id, code)
	}
	return nil
}

type podmanNetwork struct {
	ID      *string           `json:"id"`
	Name    *string           `json:"name"`
	Driver  *string           `json:"driver"`
	Labels  map[string]string `json:"labels"`
	Subnets []struct {
		Subnet  *string `json:"subnet"`
		Gateway *string `json:"gateway"`
	} `json:"subnets"`
}

func mapPodmanNetwork(n podmanNetwork) runtime.Network {
	id := derefStr(n.ID)
	if len(id) > 12 {
		id = id[:12]
	}

	subnets := make([]runtime.NetworkSubnet, 0)
	for _, s := range n.Subnets {
		subnets = append(
			subnets, runtime.NetworkSubnet{
				Subnet:  derefStr(s.Subnet),
				Gateway: derefStr(s.Gateway),
			},
		)
	}

	labels := n.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	return runtime.Network{
		ID:     id,
		Name:   derefStr(n.Name),
		Driver: derefStr(n.Driver),
		Scope:  "local",
		IPAM:   subnets,
		Labels: labels,
	}
}
