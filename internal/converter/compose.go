package converter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tidefly-oss/tidefly-plane/internal/manifest"
	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string              `yaml:"image"`
	Ports       []string            `yaml:"ports"`
	Environment any                 `yaml:"environment"`
	Volumes     []string            `yaml:"volumes"`
	Restart     string              `yaml:"restart"`
	DependsOn   any                 `yaml:"depends_on"`
	Labels      map[string]string   `yaml:"labels"`
	Command     string              `yaml:"command"`
	Healthcheck *composeHealthcheck `yaml:"healthcheck"`
}

type composeHealthcheck struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval"`
	Timeout  string   `yaml:"timeout"`
	Retries  int      `yaml:"retries"`
}

func convertCompose(input ConvertInput) (*Result, error) {
	if input.ComposeYAML == "" {
		return nil, fmt.Errorf("compose converter: compose yaml is required")
	}

	var cf composeFile
	if err := yaml.Unmarshal([]byte(input.ComposeYAML), &cf); err != nil {
		return nil, fmt.Errorf("compose converter: parse yaml: %w", err)
	}
	if len(cf.Services) == 0 {
		return nil, fmt.Errorf("compose converter: no services found")
	}

	stackName := input.StackName
	if stackName == "" {
		stackName = input.Name
	}
	if stackName == "" {
		stackName = "stack"
	}

	var manifests []*manifest.ServiceManifest

	for svcName, svc := range cf.Services {
		if svc.Image == "" {
			continue
		}

		name := fmt.Sprintf("%s-%s", stackName, svcName)
		env := parseComposeEnv(svc.Environment)
		volumes := parseComposeVolumes(svc.Volumes, stackName)
		dependsOn := parseComposeDependsOn(svc.DependsOn)

		restart := svc.Restart
		if restart == "" || restart == "unless-stopped" || restart == "on-failure" {
			restart = "always"
		}

		m := &manifest.ServiceManifest{
			APIVersion: apiVersion,
			Kind:       kindService,
			Metadata: manifest.Metadata{
				Name: name,
				Labels: map[string]string{
					"tidefly.source":    string(SourceCompose),
					"tidefly.stack":     stackName,
					"tidefly.stack-svc": svcName,
				},
			},
			Spec: manifest.ServiceSpec{
				Container: manifest.ContainerSpec{
					Image:   svc.Image,
					Env:     env,
					Volumes: volumes,
				},
				Scaling: &manifest.ScalingSpec{
					Replicas: 1,
					Restart:  restart,
					Strategy: "rolling",
				},
				DependsOn: dependsOn,
			},
		}

		if input.Expose && len(svc.Ports) > 0 {
			m.Spec.Expose = &manifest.ExposeSpec{
				Port:   firstContainerPort(svc.Ports),
				Domain: input.Domain,
				TLS:    true,
			}
		}

		if svc.Healthcheck != nil && len(svc.Healthcheck.Test) > 0 {
			m.Spec.Health = &manifest.HealthSpec{
				Interval: svc.Healthcheck.Interval,
				Timeout:  svc.Healthcheck.Timeout,
				Retries:  svc.Healthcheck.Retries,
			}
		}

		manifests = append(manifests, m)
	}

	if len(manifests) == 0 {
		return nil, fmt.Errorf("compose converter: no deployable services (all require a build step)")
	}

	return &Result{Manifests: manifests}, nil
}

func parseComposeEnv(raw any) []manifest.EnvVar {
	if raw == nil {
		return nil
	}
	var result []manifest.EnvVar
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			parts := strings.SplitN(s, "=", 2)
			if len(parts) == 2 {
				result = append(result, manifest.EnvVar{Name: parts[0], Value: parts[1]})
			} else {
				result = append(result, manifest.EnvVar{Name: parts[0]})
			}
		}
	case map[string]any:
		for key, val := range v {
			if val == nil {
				result = append(result, manifest.EnvVar{Name: key})
			} else {
				result = append(result, manifest.EnvVar{Name: key, Value: fmt.Sprintf("%v", val)})
			}
		}
	}
	return result
}

func parseComposeVolumes(volumes []string, stackName string) []manifest.VolumeSpec {
	var result []manifest.VolumeSpec
	for _, v := range volumes {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name, mountPath := parts[0], parts[1]
		if !strings.HasPrefix(name, "/") {
			name = fmt.Sprintf("%s_%s", stackName, name)
		}
		result = append(result, manifest.VolumeSpec{Name: name, MountPath: mountPath})
	}
	return result
}

func parseComposeDependsOn(raw any) []string {
	if raw == nil {
		return nil
	}
	var result []string
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	case map[string]any:
		for key := range v {
			result = append(result, key)
		}
	}
	return result
}

func firstContainerPort(ports []string) int {
	for _, p := range ports {
		parts := strings.Split(p, ":")
		containerPart := strings.SplitN(parts[len(parts)-1], "/", 2)[0]
		if n, err := strconv.Atoi(containerPart); err == nil {
			return n
		}
	}
	return 0
}
