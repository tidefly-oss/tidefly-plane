package http

import (
	"context"
	"fmt"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-backend/internal/version"
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

type OverviewInput struct{}
type OverviewOutput struct {
	Body struct {
		Containers struct {
			Total   int `json:"total"`
			Running int `json:"running"`
			Stopped int `json:"stopped"`
			Error   int `json:"error"`
		} `json:"containers"`
		Images struct {
			Total int `json:"total"`
		} `json:"images"`
		Volumes struct {
			Total int `json:"total"`
		} `json:"volumes"`
		Networks struct {
			Total int `json:"total"`
		} `json:"networks"`
		Resources struct {
			CPUPercent float64 `json:"cpu_percent"`
			Memory     struct {
				UsedMB  int64   `json:"used_mb"`
				TotalMB int64   `json:"total_mb"`
				Percent float64 `json:"percent"`
			} `json:"memory"`
			Disk struct {
				UsedMB  int64   `json:"used_mb"`
				TotalMB int64   `json:"total_mb"`
				Percent float64 `json:"percent"`
			} `json:"disk"`
		} `json:"resources"`
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
		eg.Go(
			func() error {
				d, err := h.runtime.GetContainer(egCtx, ct.Name)
				if err != nil {
					d, err = h.runtime.GetContainer(egCtx, ct.ID)
					if err != nil {
						return nil
					}
				}
				results[i] = inspectResult{details: d}
				return nil
			},
		)
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
			entries = append(
				entries, UsedPortEntry{
					Port: int(p.HostPort), HostIP: p.HostIP,
					ContainerID: r.details.ID, ContainerName: r.details.Name,
					Protocol: p.Protocol,
				},
			)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Port < entries[j].Port })
	out := &UsedPortsOutput{}
	out.Body.Ports = entries
	return out, nil
}

func (h *Handler) Overview(ctx context.Context, _ *OverviewInput) (*OverviewOutput, error) {
	var (
		containerList []runtime.Container
		imageList     []runtime.Image
		volumeList    []runtime.Volume
		networkList   []runtime.Network
		sysInfo       runtime.SystemInfo
	)
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error { list, err := h.runtime.ListContainers(egCtx, true); containerList = list; return err })
	eg.Go(func() error { list, err := h.runtime.ListImages(egCtx); imageList = list; return err })
	eg.Go(func() error { list, err := h.runtime.ListVolumes(egCtx); volumeList = list; return err })
	eg.Go(func() error { list, err := h.runtime.ListNetworks(egCtx); networkList = list; return err })
	eg.Go(func() error { info, err := h.runtime.SystemInfo(egCtx); sysInfo = info; return err })
	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("fetch overview: %w", err)
	}
	out := &OverviewOutput{}
	out.Body.Containers.Total = len(containerList)
	for _, ct := range containerList {
		switch ct.State {
		case "running":
			out.Body.Containers.Running++
		case "exited", "stopped":
			out.Body.Containers.Stopped++
		default:
			out.Body.Containers.Error++
		}
	}
	out.Body.Images.Total = len(imageList)
	out.Body.Volumes.Total = len(volumeList)
	out.Body.Networks.Total = len(networkList)
	out.Body.Resources.CPUPercent = sysInfo.CPUPercent
	out.Body.Resources.Memory.UsedMB = sysInfo.MemUsedMB
	out.Body.Resources.Memory.TotalMB = sysInfo.MemTotalMB
	out.Body.Resources.Memory.Percent = sysInfo.MemPercent
	out.Body.Resources.Disk.UsedMB = sysInfo.DiskUsedMB
	out.Body.Resources.Disk.TotalMB = sysInfo.DiskTotalMB
	out.Body.Resources.Disk.Percent = sysInfo.DiskPercent
	return out, nil
}
