package docker

import (
	"context"
	"fmt"
	"time"

	dockervolume "github.com/docker/docker/api/types/volume"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

func (d *Runtime) ListVolumes(ctx context.Context) ([]runtime.Volume, error) {
	resp, err := d.client.VolumeList(ctx, dockervolume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker list volumes: %w", err)
	}
	result := make([]runtime.Volume, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		if v.Labels[runtime.LabelInternal] == runtime.LabelTrue {
			continue
		}
		created, _ := time.Parse(time.RFC3339, v.CreatedAt)
		result = append(
			result, runtime.Volume{
				Name:      v.Name,
				Driver:    v.Driver,
				Mountpath: v.Mountpoint,
				Labels:    v.Labels,
				CreatedAt: created,
			},
		)
	}
	return result, nil
}

func (d *Runtime) CreateVolume(ctx context.Context, name string) error {
	_, err := d.client.VolumeCreate(
		ctx, dockervolume.CreateOptions{
			Name:   name,
			Labels: map[string]string{runtime.LabelManaged: runtime.LabelTrue},
		},
	)
	return err
}

func (d *Runtime) DeleteVolume(ctx context.Context, name string) error {
	return d.client.VolumeRemove(ctx, name, false)
}
