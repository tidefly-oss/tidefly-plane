package podman

import (
	"context"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

func (p *Runtime) ListVolumes(ctx context.Context) ([]runtime.Volume, error) {
	var raw []struct {
		Name       *string           `json:"Name"`
		Driver     *string           `json:"Driver"`
		Mountpoint *string           `json:"Mountpoint"`
		Labels     map[string]string `json:"Labels"`
		CreatedAt  *string           `json:"CreatedAt"`
	}

	code, err := p.c.getJSON(ctx, "/libpod/volumes/json", nil, &raw)
	if err != nil {
		return nil, fmt.Errorf("podman list volumes: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("podman list volumes: status %d", code)
	}

	result := make([]runtime.Volume, 0, len(raw))
	for _, v := range raw {
		labels := v.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		if labels[runtime.LabelInternal] == runtime.LabelTrue {
			continue
		}

		var createdAt time.Time
		if v.CreatedAt != nil {
			createdAt, _ = time.Parse(time.RFC3339, *v.CreatedAt)
		}

		result = append(
			result, runtime.Volume{
				Name:      derefStr(v.Name),
				Driver:    derefStr(v.Driver),
				Mountpath: derefStr(v.Mountpoint),
				Labels:    labels,
				CreatedAt: createdAt,
			},
		)
	}
	return result, nil
}

func (p *Runtime) CreateVolume(ctx context.Context, name string) error {
	body := map[string]any{
		"Name":           name,
		"Driver":         "local",
		"Label":          map[string]string{"tidefly.managed": "true"},
		"IgnoreIfExists": true,
	}
	_, _, err := p.c.postExpect(ctx, "/libpod/volumes/create", nil, body, 201)
	if err != nil {
		return fmt.Errorf("podman create volume %q: %w", name, err)
	}
	return nil
}

func (p *Runtime) DeleteVolume(ctx context.Context, name string) error {
	code, err := p.c.delete(ctx, "/libpod/volumes/"+escPath(name), nil)
	if err != nil {
		return fmt.Errorf("podman delete volume %q: %w", name, err)
	}
	if code != 200 && code != 204 {
		return fmt.Errorf("podman delete volume %q: status %d", name, code)
	}
	return nil
}
