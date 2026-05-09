package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

type Runtime struct {
	client *client.Client
}

func New(socketPath string) (*Runtime, error) {
	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if socketPath != "" {
		opts = append(opts, client.WithHost("unix://"+socketPath))
	} else {
		opts = append(opts, client.FromEnv)
	}
	c, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Runtime{client: c}, nil
}

func (d *Runtime) ContainerStats(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := d.client.ContainerStats(ctx, id, true)
	if err != nil {
		return nil, fmt.Errorf("docker stats: %w", err)
	}
	return resp.Body, nil
}

// DockerStats — aufbereitete Stats für SSE
type DockerStats struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemUsageMB   float64 `json:"mem_usage_mb"`
	MemLimitMB   float64 `json:"mem_limit_mb"`
	MemPercent   float64 `json:"mem_percent"`
	NetworkRxMB  float64 `json:"network_rx_mb"`
	NetworkTxMB  float64 `json:"network_tx_mb"`
	BlockReadMB  float64 `json:"block_read_mb"`
	BlockWriteMB float64 `json:"block_write_mb"`
	PIDs         uint64  `json:"pids"`
}

// ParseStats rechnet rohe Docker Stats zu lesbaren Werten um.
// container.StatsResponse ist der korrekte Typ im aktuellen Docker SDK.
func ParseStats(raw []byte) (*DockerStats, error) {
	var s container.StatsResponse
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}

	// CPU %
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage - s.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(s.CPUStats.SystemUsage - s.PreCPUStats.SystemUsage)
	numCPUs := float64(s.CPUStats.OnlineCPUs)
	if numCPUs == 0 {
		numCPUs = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	// Memory
	memUsage := float64(s.MemoryStats.Usage) / 1024 / 1024
	memLimit := float64(s.MemoryStats.Limit) / 1024 / 1024
	memPercent := 0.0
	if memLimit > 0 {
		memPercent = (memUsage / memLimit) * 100.0
	}

	// Network I/O
	var rxBytes, txBytes float64
	for _, n := range s.Networks {
		rxBytes += float64(n.RxBytes)
		txBytes += float64(n.TxBytes)
	}

	// Block I/O
	var blkRead, blkWrite float64
	for _, bio := range s.BlkioStats.IoServiceBytesRecursive {
		switch bio.Op {
		case "Read":
			blkRead += float64(bio.Value)
		case "Write":
			blkWrite += float64(bio.Value)
		}
	}

	return &DockerStats{
		CPUPercent:   cpuPercent,
		MemUsageMB:   memUsage,
		MemLimitMB:   memLimit,
		MemPercent:   memPercent,
		NetworkRxMB:  rxBytes / 1024 / 1024,
		NetworkTxMB:  txBytes / 1024 / 1024,
		BlockReadMB:  blkRead / 1024 / 1024,
		BlockWriteMB: blkWrite / 1024 / 1024,
		PIDs:         s.PidsStats.Current,
	}, nil
}
