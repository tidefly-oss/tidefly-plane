package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/client"
	"gorm.io/gorm"
)

type Runtime struct {
	client *client.Client
	db     *gorm.DB // optional — nil for plain runtime without DB
}

func New(socketPath string, db *gorm.DB) (*Runtime, error) {
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
	return &Runtime{client: c, db: db}, nil
}

func (d *Runtime) ContainerStats(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := d.client.ContainerStats(ctx, id, true)
	if err != nil {
		return nil, fmt.Errorf("docker stats: %w", err)
	}
	return resp.Body, nil
}

// DockerStats — a subset of the full Docker stats JSON, with some fields converted to more convenient units.
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
