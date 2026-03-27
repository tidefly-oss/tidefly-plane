package podman

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
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
	defer func() { _ = resp.Body.Close() }()
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
	defer func() { _ = resp.Body.Close() }()
	return io.NopCloser(bytes.NewReader(b)), nil
}
