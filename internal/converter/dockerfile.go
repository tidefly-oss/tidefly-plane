package converter

import (
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
)

func convertDockerfile(input ConvertInput) (*Result, error) {
	if input.Dockerfile == "" {
		return nil, fmt.Errorf("dockerfile converter: dockerfile content is required")
	}
	if input.Name == "" {
		return nil, fmt.Errorf("dockerfile converter: name is required")
	}

	buildTag := input.BuildTag
	if buildTag == "" {
		buildTag = fmt.Sprintf("localhost/tidefly/%s:latest", input.Name)
	}
	buildTag = qualifyLocalTag(buildTag)

	m := &manifest.ServiceManifest{
		APIVersion: apiVersion,
		Kind:       kindService,
		Metadata: manifest.Metadata{
			Name: input.Name,
			Labels: map[string]string{
				"tidefly.source":    string(SourceDockerfile),
				"tidefly.build-tag": buildTag,
			},
		},
		Spec: manifest.ServiceSpec{
			Container: manifest.ContainerSpec{
				Image: buildTag,
				Env:   input.Env,
			},
		},
	}

	if input.Expose || input.Domain != "" {
		m.Spec.Expose = &manifest.ExposeSpec{
			Port:   input.Port,
			Domain: input.Domain,
			TLS:    true,
		}
	}

	if input.Replicas > 0 || input.Strategy != "" {
		replicas := input.Replicas
		if replicas == 0 {
			replicas = 1
		}
		m.Spec.Scaling = &manifest.ScalingSpec{
			Replicas: replicas,
			Strategy: input.Strategy,
			Restart:  "always",
		}
	}

	return &Result{
		Manifests:        []*manifest.ServiceManifest{m},
		BuildRequired:    true,
		BuildTag:         buildTag,
		InlineDockerfile: input.Dockerfile,
	}, nil
}

func qualifyLocalTag(tag string) string {
	parts := strings.SplitN(tag, "/", 2)
	if strings.Contains(parts[0], ".") || parts[0] == "localhost" {
		return tag
	}
	return "localhost/" + tag
}
