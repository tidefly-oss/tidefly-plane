package podman

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

func (p *Runtime) SystemInfo(ctx context.Context) (runtime.SystemInfo, error) {
	var info struct {
		Store *struct {
			ContainerStore *struct {
				Number  *int64 `json:"number"`
				Running *int64 `json:"running"`
				Paused  *int64 `json:"paused"`
				Stopped *int64 `json:"stopped"`
			} `json:"ContainerStore"`
		} `json:"store"`
	}
	if code, err := p.c.getJSON(ctx, "/libpod/info", nil, &info); err != nil || code != 200 {
		return runtime.SystemInfo{}, fmt.Errorf("podman system info: status %d: %w", code, err)
	}

	var ver struct {
		Version    *string `json:"Version"`
		APIVersion *string `json:"APIVersion"`
		Os         *string `json:"Os"`
		Arch       *string `json:"Arch"`
	}
	_, _ = p.c.getJSON(ctx, "/libpod/version", nil, &ver)

	si := runtime.SystemInfo{RuntimeType: runtime.RuntimePodman}

	if ver.Version != nil {
		si.Version = *ver.Version
	}
	if ver.APIVersion != nil {
		si.APIVersion = *ver.APIVersion
	}
	if ver.Os != nil {
		si.OS = *ver.Os
	}
	if ver.Arch != nil {
		si.Architecture = *ver.Arch
	}

	if info.Store != nil && info.Store.ContainerStore != nil {
		cs := info.Store.ContainerStore
		if cs.Number != nil {
			si.ContainerCount = int(*cs.Number)
		}
		if cs.Running != nil {
			si.RunningCount = int(*cs.Running)
		}
		if cs.Paused != nil {
			si.PausedCount = int(*cs.Paused)
		}
		if cs.Stopped != nil {
			si.StoppedCount = int(*cs.Stopped)
		}
	}

	if cpuPcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(cpuPcts) > 0 {
		si.CPUPercent = cpuPcts[0]
	}
	if vmStat, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		si.TotalMemory = int64(vmStat.Total)
		si.MemTotalMB = int64(vmStat.Total / 1024 / 1024)
		si.MemUsedMB = int64(vmStat.Used / 1024 / 1024)
		si.MemPercent = vmStat.UsedPercent
	}
	if diskStat, err := disk.UsageWithContext(ctx, "/"); err == nil {
		si.DiskTotalMB = int64(diskStat.Total / 1024 / 1024)
		si.DiskUsedMB = int64(diskStat.Used / 1024 / 1024)
		si.DiskPercent = diskStat.UsedPercent
	}
	if uptime, err := host.Uptime(); err == nil {
		si.UptimeSeconds = uptime
	}

	return si, nil
}
