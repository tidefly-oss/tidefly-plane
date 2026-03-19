package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerfilters "github.com/docker/docker/api/types/filters"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

func (d *Runtime) ListImages(ctx context.Context) ([]runtime.Image, error) {
	list, err := d.client.ImageList(ctx, dockerimage.ListOptions{Filters: dockerfilters.Args{}})
	if err != nil {
		return nil, fmt.Errorf("docker list images: %w", err)
	}
	internalImages := map[string]bool{}
	if all, err := d.client.ContainerList(ctx, container.ListOptions{All: true}); err == nil {
		for _, ct := range all {
			if ct.Labels[runtime.LabelInternal] == runtime.LabelTrue {
				internalImages[strings.TrimPrefix(ct.Image, "docker.io/")] = true
			}
		}
	}
	result := make([]runtime.Image, 0, len(list))
	for _, img := range list {
		if !hasRealTag(img.RepoTags) || isInternalImage(img.RepoTags, internalImages) {
			continue
		}
		id := img.ID
		if len(id) > 19 {
			id = id[7:19]
		}
		result = append(
			result, runtime.Image{
				ID:      id,
				Tags:    img.RepoTags,
				Size:    img.Size,
				Created: time.Unix(img.Created, 0),
			},
		)
	}
	return result, nil
}

func (d *Runtime) DeleteImage(ctx context.Context, id string, force bool) error {
	_, err := d.client.ImageRemove(ctx, id, dockerimage.RemoveOptions{Force: force})
	return err
}

func hasRealTag(tags []string) bool {
	for _, t := range tags {
		if t != "" && t != "<none>" && t != "<none>:<none>" {
			return true
		}
	}
	return false
}

func isInternalImage(tags []string, internal map[string]bool) bool {
	for _, t := range tags {
		if internal[t] {
			return true
		}
	}
	return false
}
