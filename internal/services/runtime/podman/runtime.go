package podman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

type Runtime struct {
	socketPath string
	c          *client
}

func New(socketPath string) (*Runtime, error) {
	if socketPath == "" {
		socketPath = "/run/user/1000/podman/podman.sock"
	}
	return &Runtime{
		socketPath: socketPath,
		c:          newClient(socketPath),
	}, nil
}

func (p *Runtime) Type() runtime.RuntimeType { return runtime.RuntimePodman }

func (p *Runtime) Ping(ctx context.Context) error {
	resp, err := p.c.get(ctx, "/libpod/_ping", nil)
	if err != nil {
		return fmt.Errorf("podman ping: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("podman ping: status %d", resp.StatusCode)
	}
	return nil
}

func (p *Runtime) ContainerStats(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := p.c.get(ctx, "/libpod/containers/"+escPath(id)+"/stats", nil)
	if err != nil {
		return nil, fmt.Errorf("podman stats: %w", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return io.NopCloser(bytes.NewReader(b)), nil
}

type PodmanStats struct {
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

func ParseStats(raw []byte) (*PodmanStats, error) {
	var s map[string]interface{}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("unmarshal podman stats: %w", err)
	}

	ds := &PodmanStats{}

	if cpu, ok := s["cpu"].(map[string]interface{}); ok {
		totalUsage, _ := cpu["total_usage"].(float64)
		systemUsage, _ := cpu["system_cpu_usage"].(float64)
		onlineCPUs, _ := cpu["online_cpus"].(float64)
		if systemUsage > 0 {
			ds.CPUPercent = (totalUsage / systemUsage) * onlineCPUs * 100
		}
	}

	if mem, ok := s["memory"].(map[string]interface{}); ok {
		usage, _ := mem["usage"].(float64)
		limit, _ := mem["limit"].(float64)
		ds.MemUsageMB = usage / 1024 / 1024
		ds.MemLimitMB = limit / 1024 / 1024
		if limit > 0 {
			ds.MemPercent = ds.MemUsageMB / ds.MemLimitMB * 100
		}
	}

	var rx, tx float64
	if nets, ok := s["networks"].(map[string]interface{}); ok {
		for _, v := range nets {
			if n, ok := v.(map[string]interface{}); ok {
				r, _ := n["rx_bytes"].(float64)
				t, _ := n["tx_bytes"].(float64)
				rx += r
				tx += t
			}
		}
	}
	ds.NetworkRxMB = rx / 1024 / 1024
	ds.NetworkTxMB = tx / 1024 / 1024

	var blkRead, blkWrite float64
	if blkios, ok := s["blkio"].([]interface{}); ok {
		for _, b := range blkios {
			if bio, ok := b.(map[string]interface{}); ok {
				op, _ := bio["op"].(string)
				val, _ := bio["value"].(float64)
				switch op {
				case "Read":
					blkRead += val
				case "Write":
					blkWrite += val
				}
			}
		}
	}
	ds.BlockReadMB = blkRead / 1024 / 1024
	ds.BlockWriteMB = blkWrite / 1024 / 1024

	if pids, ok := s["pids_stats"].(map[string]interface{}); ok {
		if cur, ok := pids["current"].(float64); ok {
			ds.PIDs = uint64(cur)
		}
	}

	return ds, nil
}
