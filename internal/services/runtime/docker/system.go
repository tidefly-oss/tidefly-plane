package docker

import (
	"context"
	"fmt"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

func (d *Runtime) Type() runtime.RuntimeType { return runtime.RuntimeDocker }

func (d *Runtime) Ping(ctx context.Context) error {
	_, err := d.client.Ping(ctx)
	return err
}

func (d *Runtime) SystemInfo(ctx context.Context) (runtime.SystemInfo, error) {
	info, err := d.client.Info(ctx)
	if err != nil {
		return runtime.SystemInfo{}, fmt.Errorf("docker info: %w", err)
	}
	ver, err := d.client.ServerVersion(ctx)
	if err != nil {
		return runtime.SystemInfo{}, fmt.Errorf("docker version: %w", err)
	}
	si := runtime.SystemInfo{
		RuntimeType:    runtime.RuntimeDocker,
		Version:        ver.Version,
		APIVersion:     ver.APIVersion,
		OS:             info.OSType,
		Architecture:   info.Architecture,
		TotalMemory:    info.MemTotal,
		ContainerCount: info.Containers,
		RunningCount:   info.ContainersRunning,
		PausedCount:    info.ContainersPaused,
		StoppedCount:   info.ContainersStopped,
	}
	if cpuPcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(cpuPcts) > 0 {
		si.CPUPercent = cpuPcts[0]
	}
	if vmStat, err := mem.VirtualMemoryWithContext(ctx); err == nil {
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
