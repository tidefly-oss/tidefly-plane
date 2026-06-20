package jobs

import (
	"context"
	"encoding/json"
	"fmt"
)

type containerMetrics struct {
	cpuPercent float64
	memPercent float64
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			Cache uint64 `json:"cache"`
		} `json:"stats"`
	} `json:"memory_stats"`
}

type podmanStats struct {
	CPU     float64 `json:"CPU"`
	MemPerc float64 `json:"MemPerc"`
}

func (h *ServiceJobHandler) readStats(ctx context.Context, id string) (*containerMetrics, error) {
	rc, err := h.rt.ContainerStats(ctx, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	data := make([]byte, 65536)
	n, _ := rc.Read(data)
	data = data[:n]

	var ds dockerStats
	if err := json.Unmarshal(data, &ds); err == nil && ds.CPUStats.SystemCPUUsage > 0 {
		cpuDelta := float64(ds.CPUStats.CPUUsage.TotalUsage) - float64(ds.PreCPUStats.CPUUsage.TotalUsage)
		sysDelta := float64(ds.CPUStats.SystemCPUUsage) - float64(ds.PreCPUStats.SystemCPUUsage)
		cpus := ds.CPUStats.OnlineCPUs
		if cpus == 0 {
			cpus = 1
		}
		var cpu float64
		if sysDelta > 0 {
			cpu = (cpuDelta / sysDelta) * float64(cpus) * 100
		}
		var mem float64
		if ds.MemoryStats.Limit > 0 {
			mem = float64(ds.MemoryStats.Usage-ds.MemoryStats.Stats.Cache) / float64(ds.MemoryStats.Limit) * 100
		}
		return &containerMetrics{cpuPercent: cpu, memPercent: mem}, nil
	}

	var ps podmanStats
	if err := json.Unmarshal(data, &ps); err == nil {
		return &containerMetrics{cpuPercent: ps.CPU, memPercent: ps.MemPerc}, nil
	}

	return nil, fmt.Errorf("unknown stats format")
}
