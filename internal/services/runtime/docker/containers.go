package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	dockermount "github.com/docker/docker/api/types/mount"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

func (d *Runtime) ListContainers(ctx context.Context, all bool) ([]runtime.Container, error) {
	list, err := d.client.ContainerList(ctx, container.ListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("docker list containers: %w", err)
	}
	result := make([]runtime.Container, 0, len(list))
	for _, c := range list {
		if c.Labels["tidefly.internal"] == "true" {
			continue
		}
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0][1:]
		}
		ports := make([]runtime.Port, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(
				ports, runtime.Port{
					HostIP:        p.IP,
					HostPort:      p.PublicPort,
					ContainerPort: p.PrivatePort,
					Protocol:      p.Type,
				},
			)
		}
		mounts := make([]runtime.Mount, 0, len(c.Mounts))
		for _, m := range c.Mounts {
			mounts = append(
				mounts, runtime.Mount{
					Source:      m.Name,
					Destination: m.Destination,
					Mode:        m.Mode,
					RW:          m.RW,
				},
			)
		}
		networks := make([]string, 0, len(c.NetworkSettings.Networks))
		for n := range c.NetworkSettings.Networks {
			networks = append(networks, n)
		}
		result = append(
			result, runtime.Container{
				ID:       c.ID[:12],
				Name:     name,
				Image:    c.Image,
				Status:   mapStatus(c.State),
				State:    c.State,
				Created:  time.Unix(c.Created, 0),
				Ports:    ports,
				Labels:   c.Labels,
				Mounts:   mounts,
				Networks: networks,
			},
		)
	}
	return result, nil
}

func (d *Runtime) GetContainer(ctx context.Context, id string) (*runtime.ContainerDetails, error) {
	inspect, err := d.client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("docker inspect container: %w", err)
	}
	created, _ := time.Parse(time.RFC3339Nano, inspect.Created)
	name := inspect.Name
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	ports := make([]runtime.Port, 0)
	for containerPort, bindings := range (map[nat.Port][]nat.PortBinding)(inspect.NetworkSettings.Ports) {
		for _, b := range bindings {
			p, err := strconv.ParseUint(b.HostPort, 10, 16)
			if err != nil {
				continue
			}
			ports = append(
				ports, runtime.Port{
					HostIP:        b.HostIP,
					HostPort:      uint16(p),
					ContainerPort: uint16(containerPort.Int()),
					Protocol:      containerPort.Proto(),
				},
			)
		}
	}
	mounts := make([]runtime.Mount, len(inspect.Mounts))
	for i, m := range inspect.Mounts {
		mounts[i] = runtime.Mount{Source: m.Source, Destination: m.Destination, Mode: m.Mode, RW: m.RW}
	}
	networks := make([]string, 0, len(inspect.NetworkSettings.Networks))
	for n := range inspect.NetworkSettings.Networks {
		networks = append(networks, n)
	}
	return &runtime.ContainerDetails{
		Container: runtime.Container{
			ID:      inspect.ID[:12],
			Name:    name,
			Image:   inspect.Config.Image,
			Status:  mapStatus(inspect.State.Status),
			State:   inspect.State.Status,
			Created: created,
			Ports:   ports,
			Labels:  inspect.Config.Labels,
		},
		Command:       inspect.Path,
		Entrypoint:    inspect.Config.Entrypoint,
		Env:           inspect.Config.Env,
		Mounts:        mounts,
		Networks:      networks,
		RestartPolicy: string(inspect.HostConfig.RestartPolicy.Name),
	}, nil
}

// CreateContainer creates and immediately starts a container from a ContainerSpec.
func (d *Runtime) CreateContainer(ctx context.Context, spec runtime.ContainerSpec) error {
	// Auto-pull image if not present locally
	_, err := d.client.ImageInspect(ctx, spec.Image)
	if err != nil {
		reader, pullErr := d.client.ImagePull(ctx, spec.Image, image.PullOptions{})
		if pullErr != nil {
			return fmt.Errorf("docker pull image %q: %w", spec.Image, pullErr)
		}
		_, _ = io.Copy(io.Discard, reader)
		_ = reader.Close()
	}

	// Remove existing container with same name if present
	existing, inspectErr := d.client.ContainerInspect(ctx, spec.Name)
	if inspectErr == nil {
		_ = d.client.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true})
	}

	// Port bindings
	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}
	for _, p := range spec.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		cp := nat.Port(fmt.Sprintf("%d/%s", p.ContainerPort, proto))
		portBindings[cp] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: p.HostPort}}
		exposedPorts[cp] = struct{}{}
	}

	// Volume mounts
	mounts := make([]dockermount.Mount, 0, len(spec.Volumes))
	for _, v := range spec.Volumes {
		mounts = append(
			mounts, dockermount.Mount{
				Type:   dockermount.TypeVolume,
				Source: v.Name,
				Target: v.Mount,
			},
		)
	}

	// Healthcheck
	var hc *container.HealthConfig
	if spec.Healthcheck != nil {
		hc = &container.HealthConfig{
			Test:        spec.Healthcheck.Test,
			Interval:    spec.Healthcheck.Interval,
			Timeout:     spec.Healthcheck.Timeout,
			Retries:     spec.Healthcheck.Retries,
			StartPeriod: spec.Healthcheck.StartPeriod,
		}
	}

	// Restart policy
	restart := spec.Restart
	if restart == "" {
		restart = "unless-stopped"
	}

	// Optional shell command
	var cmd []string
	if spec.Command != "" {
		cmd = []string{"sh", "-c", spec.Command}
	}

	resp, err := d.client.ContainerCreate(
		ctx,
		&container.Config{
			Image:        spec.Image,
			Env:          spec.Env,
			Labels:       spec.Labels,
			ExposedPorts: exposedPorts,
			Healthcheck:  hc,
			Cmd:          cmd,
		},
		&container.HostConfig{
			PortBindings:  portBindings,
			Mounts:        mounts,
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(restart)},
			NetworkMode:   container.NetworkMode(spec.Network),
		},
		&dockernetwork.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetwork.EndpointSettings{
				spec.Network: {},
			},
		},
		nil,
		spec.Name,
	)
	if err != nil {
		return fmt.Errorf("docker create container %q: %w", spec.Name, err)
	}

	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker start container %q: %w", spec.Name, err)
	}
	return nil
}

func (d *Runtime) StartContainer(ctx context.Context, id string) error {
	return d.client.ContainerStart(ctx, id, container.StartOptions{})
}

func (d *Runtime) StopContainer(ctx context.Context, id string, opts runtime.StopOptions) error {
	return d.client.ContainerStop(ctx, id, container.StopOptions{Timeout: opts.Timeout})
}

func (d *Runtime) RestartContainer(ctx context.Context, id string, opts runtime.StopOptions) error {
	return d.client.ContainerRestart(ctx, id, container.StopOptions{Timeout: opts.Timeout})
}

func (d *Runtime) DeleteContainer(ctx context.Context, id string, force bool) error {
	return d.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: force})
}

func (d *Runtime) ContainerLogs(ctx context.Context, id string, opts runtime.LogOptions) (io.ReadCloser, error) {
	return d.client.ContainerLogs(
		ctx, id, container.LogsOptions{
			ShowStdout: true, ShowStderr: true,
			Follow: opts.Follow, Tail: opts.Tail, Since: opts.Since, Timestamps: opts.Timestamps,
		},
	)
}
