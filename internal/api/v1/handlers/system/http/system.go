package http

import (
	"context"
	"fmt"
	"sort"

	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/version"
	"golang.org/x/sync/errgroup"
)

type HealthInput struct{}
type HealthOutput struct {
	Body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
}

type InfoInput struct{}
type InfoOutput struct {
	Body struct {
		runtime.SystemInfo
		TideflyVersion string `json:"tidefly_version"`
	}
}

type UsedPortEntry struct {
	Port          int    `json:"port"`
	HostIP        string `json:"host_ip"`
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	Protocol      string `json:"protocol"`
}

type UsedPortsInput struct{}
type UsedPortsOutput struct {
	Body struct {
		Ports []UsedPortEntry `json:"ports"`
	}
}

func (h *Handler) Health(_ context.Context, _ *HealthInput) (*HealthOutput, error) {
	out := &HealthOutput{}
	out.Body.Status = "ok"
	out.Body.Version = version.Version
	return out, nil
}

func (h *Handler) Info(ctx context.Context, _ *InfoInput) (*InfoOutput, error) {
	info, err := h.runtime.SystemInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("system info: %w", err)
	}
	out := &InfoOutput{}
	out.Body.SystemInfo = info
	out.Body.TideflyVersion = version.Version
	return out, nil
}

func (h *Handler) UsedPorts(ctx context.Context, _ *UsedPortsInput) (*UsedPortsOutput, error) {
	allContainers, err := h.runtime.ListAllContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	type inspectResult struct{ details *runtime.ContainerDetails }
	results := make([]inspectResult, len(allContainers))
	eg, egCtx := errgroup.WithContext(ctx)
	for i, ct := range allContainers {
		i, ct := i, ct
		eg.Go(func() error {
			d, err := h.runtime.GetContainer(egCtx, ct.Name)
			if err != nil {
				d, err = h.runtime.GetContainer(egCtx, ct.ID)
				if err != nil {
					return nil
				}
			}
			results[i] = inspectResult{details: d}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("inspect containers: %w", err)
	}
	var entries []UsedPortEntry
	seen := make(map[int]bool)
	for _, r := range results {
		if r.details == nil {
			continue
		}
		for _, p := range r.details.Ports {
			if p.HostPort == 0 || seen[int(p.HostPort)] {
				continue
			}
			seen[int(p.HostPort)] = true
			entries = append(entries, UsedPortEntry{
				Port: int(p.HostPort), HostIP: p.HostIP,
				ContainerID: r.details.ID, ContainerName: r.details.Name,
				Protocol: p.Protocol,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Port < entries[j].Port })
	out := &UsedPortsOutput{}
	out.Body.Ports = entries
	return out, nil
}
