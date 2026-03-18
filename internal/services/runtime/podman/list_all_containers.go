package podman

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

// ListAllContainers returns every container on the host, including those
// marked tidefly.internal. Used for port-conflict detection.
func (p *Runtime) ListAllContainers(ctx context.Context) ([]runtime.Container, error) {
	var raw []struct {
		ID    *string  `json:"Id"`
		Names []string `json:"Names"`
		Image *string  `json:"Image"`
		State *string  `json:"State"`
	}

	if _, err := p.c.getJSON(ctx, "/libpod/containers/json?all=true", nil, &raw); err != nil {
		return nil, fmt.Errorf("podman list all containers: %w", err)
	}

	result := make([]runtime.Container, 0, len(raw))
	for _, c := range raw {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		result = append(
			result, runtime.Container{
				ID:    derefStr(c.ID), // full ID for reliable Inspect
				Name:  name,
				Image: derefStr(c.Image),
				State: derefStr(c.State),
			},
		)
	}
	return result, nil
}
