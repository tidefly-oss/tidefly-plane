package podman

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

func (p *Runtime) ListImages(ctx context.Context) ([]runtime.Image, error) {
	// Collect image IDs used by internal (tidefly.internal=true) containers
	// so we can hide them from the image list.
	internalImageIDs, err := p.internalImageIDs(ctx)
	if err != nil {
		// Non-fatal — worst case we show internal images
		internalImageIDs = map[string]struct{}{}
	}

	var raw []struct {
		ID       *string  `json:"Id"`
		RepoTags []string `json:"RepoTags"`
		Size     *int64   `json:"Size"`
		Created  *int64   `json:"Created"`
	}

	code, err := p.c.getJSON(ctx, "/libpod/images/json", nil, &raw)
	if err != nil {
		return nil, fmt.Errorf("podman list images: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("podman list images: status %d", code)
	}

	result := make([]runtime.Image, 0, len(raw))
	for _, img := range raw {
		tags := img.RepoTags
		if !podmanHasRealTag(tags) {
			continue
		}

		id := derefStr(img.ID)
		id = stripSHA(id)

		// Skip images exclusively used by internal tidefly containers
		if _, internal := internalImageIDs[id]; internal {
			continue
		}

		var created time.Time
		if img.Created != nil {
			created = time.Unix(*img.Created, 0)
		}

		size := int64(0)
		if img.Size != nil {
			size = *img.Size
		}

		result = append(
			result, runtime.Image{
				ID:      id,
				Tags:    tags,
				Size:    size,
				Created: created,
			},
		)
	}
	return result, nil
}

// internalImageIDs returns the set of image IDs (short SHA) that are used
// exclusively by containers labelled tidefly.internal=true. Images used by
// at least one non-internal container are NOT included so they remain visible.
func (p *Runtime) internalImageIDs(ctx context.Context) (map[string]struct{}, error) {
	var raw []struct {
		ImageID string            `json:"ImageID"`
		Labels  map[string]string `json:"Labels"`
	}

	q := url.Values{}
	q.Set("all", "true")

	code, err := p.c.getJSON(ctx, "/libpod/containers/json", q, &raw)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("status %d", code)
	}

	// Track how many containers use each image, and how many of those are internal.
	type counts struct{ total, internal int }
	usage := map[string]*counts{}

	for _, c := range raw {
		imgID := stripSHA(c.ImageID)
		if imgID == "" {
			continue
		}
		if usage[imgID] == nil {
			usage[imgID] = &counts{}
		}
		usage[imgID].total++
		if c.Labels["tidefly.internal"] == "true" {
			usage[imgID].internal++
		}
	}

	result := make(map[string]struct{})
	for imgID, cnt := range usage {
		// Only hide if ALL containers using this image are internal
		if cnt.total > 0 && cnt.total == cnt.internal {
			result[imgID] = struct{}{}
		}
	}
	return result, nil
}

func (p *Runtime) DeleteImage(ctx context.Context, id string, force bool) error {
	q := url.Values{}
	q.Set("force", fmt.Sprintf("%v", force))
	code, err := p.c.delete(ctx, "/libpod/images/"+escPath(id), q)
	if err != nil {
		return fmt.Errorf("podman delete image %q: %w", id, err)
	}
	if code != 200 && code != 204 {
		return fmt.Errorf("podman delete image %q: status %d", id, code)
	}
	return nil
}

func podmanHasRealTag(tags []string) bool {
	for _, t := range tags {
		if t != "" && !strings.Contains(t, "<none>") {
			return true
		}
	}
	return false
}
