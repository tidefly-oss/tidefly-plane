package converter

import (
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
)

func convertImage(input ConvertInput) (*Result, error) {
	if input.Image == "" {
		return nil, fmt.Errorf("image converter: image is required")
	}

	name := input.Name
	if name == "" {
		name = imageToName(input.Image)
	}

	m := &manifest.ServiceManifest{
		APIVersion: apiVersion,
		Kind:       kindService,
		Metadata: manifest.Metadata{
			Name:   name,
			Labels: map[string]string{"tidefly.source": string(SourceImage)},
		},
		Spec: manifest.ServiceSpec{
			Container: manifest.ContainerSpec{
				Image: input.Image,
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

	return &Result{Manifests: []*manifest.ServiceManifest{m}}, nil
}

func imageToName(image string) string {
	base := strings.SplitN(image, ":", 2)[0]
	parts := strings.Split(base, "/")
	return parts[len(parts)-1]
}
