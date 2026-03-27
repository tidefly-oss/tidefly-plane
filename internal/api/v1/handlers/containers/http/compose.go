package http

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
	"gopkg.in/yaml.v3"
)

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
	Volumes  map[string]any            `yaml:"volumes"`
	Networks map[string]any            `yaml:"networks"`
}

type composeService struct {
	Image       string            `yaml:"image"`
	Build       any               `yaml:"build"`
	Ports       []string          `yaml:"ports"`
	Environment any               `yaml:"environment"`
	Volumes     []string          `yaml:"volumes"`
	Restart     string            `yaml:"restart"`
	DependsOn   any               `yaml:"depends_on"`
	Labels      map[string]string `yaml:"labels"`
	Command     string            `yaml:"command"`
	Networks    any               `yaml:"networks"`
}

type DeployComposeInput struct {
	Body struct {
		Compose   string `json:"compose"     minLength:"1"   doc:"Docker Compose YAML"`
		StackName string `json:"stack_name"  minLength:"1"   maxLength:"128" doc:"Stack name"`
		ProjectID string `json:"project_id"  format:"uuid"   doc:"Project ID"`
		Expose    bool   `json:"expose"      doc:"Route all HTTP services via Caddy"`
	}
}

type DeployComposeOutput struct {
	Body struct {
		StackID    string            `json:"stack_id"`
		Containers []string          `json:"containers"`
		URLs       map[string]string `json:"urls,omitempty"`
	}
}

func (h *Handler) DeployCompose(ctx context.Context, input *DeployComposeInput) (*DeployComposeOutput, error) {
	if input.Body.Expose && !h.CaddyEnabled() {
		return nil, huma.Error400BadRequest("Caddy integration is not enabled on this instance")
	}

	project, err := h.projects.GetByID(input.Body.ProjectID)
	if err != nil {
		return nil, huma404("project not found")
	}

	var cf composeFile
	if err := yaml.Unmarshal([]byte(input.Body.Compose), &cf); err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("invalid compose file: %s", err))
	}
	if len(cf.Services) == 0 {
		return nil, huma.Error400BadRequest("no services found in compose file")
	}

	stackID := uuid.New().String()
	var created []string
	urls := make(map[string]string)

	for svcName, svc := range cf.Services {
		if svc.Image == "" {
			continue
		}
		containerName := fmt.Sprintf("%s-%s", input.Body.StackName, svcName)
		labels := map[string]string{
			"tidefly.managed":    "true",
			"tidefly.stack_id":   stackID,
			"tidefly.stack_name": input.Body.StackName,
			"tidefly.source":     "compose",
			"tidefly.service":    svcName,
			"tidefly.project":    input.Body.ProjectID,
		}
		for k, v := range svc.Labels {
			labels[k] = v
		}

		ports := parseComposePorts(svc.Ports)
		restart := svc.Restart
		if restart == "" {
			restart = "unless-stopped"
		}

		spec := runtime.ContainerSpec{
			Name:    containerName,
			Image:   svc.Image,
			Env:     parseComposeEnv(svc.Environment),
			Ports:   ports,
			Volumes: parseComposeVolumes(svc.Volumes, input.Body.StackName),
			Labels:  labels,
			Restart: restart,
			Command: svc.Command,
			Network: project.NetworkName,
		}

		// When exposing via Caddy — no host port binding needed
		if input.Body.Expose && len(ports) > 0 {
			spec.Ports = nil
		}

		if err := h.runtime.CreateContainer(ctx, spec); err != nil {
			for _, name := range created {
				_ = h.runtime.DeleteContainer(ctx, name, true)
			}
			h.log.Audit(
				ctx, logger.AuditEntry{
					Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: false,
					Details: fmt.Sprintf(
						"compose stack=%q create=%q rolled_back=%d err=%s",
						input.Body.StackName,
						containerName,
						len(created),
						err,
					),
				},
			)
			return nil, fmt.Errorf("create container %q: %w", containerName, err)
		}
		if err := h.runtime.StartContainer(ctx, containerName); err != nil {
			for _, name := range created {
				_ = h.runtime.DeleteContainer(ctx, name, true)
			}
			h.log.Audit(
				ctx, logger.AuditEntry{
					Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: false,
					Details: fmt.Sprintf(
						"compose stack=%q start=%q rolled_back=%d err=%s",
						input.Body.StackName,
						containerName,
						len(created),
						err,
					),
				},
			)
			return nil, fmt.Errorf("start container %q: %w", containerName, err)
		}

		if input.Body.Expose {
			if err := h.runtime.ConnectNetwork(ctx, containerName, "tidefly_proxy"); err != nil {
				h.log.Warn("caddy", "failed to connect to proxy network", err)
			}
		}

		// Register Caddy route if expose=true and service has a port
		if input.Body.Expose && h.CaddyEnabled() && len(ports) > 0 {
			domain := caddysvc.Domain(h.caddy.Config(), containerName)
			upstream := fmt.Sprintf("%s:%d", containerName, ports[0].ContainerPort)
			routeID := caddysvc.RouteID(containerName)
			if err := h.caddy.AddHTTPRoute(ctx, routeID, domain, upstream); err != nil {
				h.log.Error("caddy", "failed to register route for "+containerName, err)
			} else {
				urls[svcName] = "https://" + domain
			}
		}

		created = append(created, containerName)
	}

	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: true,
			Details: fmt.Sprintf(
				"compose stack=%q project=%s containers=%d expose=%v",
				input.Body.StackName,
				input.Body.ProjectID,
				len(created),
				input.Body.Expose,
			),
		},
	)

	out := &DeployComposeOutput{}
	out.Body.StackID = stackID
	out.Body.Containers = created
	if len(urls) > 0 {
		out.Body.URLs = urls
	}
	return out, nil
}

// ── DeleteStack ───────────────────────────────────────────────────────────────

type DeleteStackInput struct {
	StackID string `path:"stack_id" doc:"Stack ID"`
}

func (h *Handler) DeleteStack(ctx context.Context, input *DeleteStackInput) (*struct{}, error) {
	all, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	var deleted []string
	for _, ct := range all {
		if ct.Labels["tidefly.stack_id"] != input.StackID {
			continue
		}
		// Remove Caddy route before deleting container
		if h.CaddyEnabled() {
			_ = h.caddy.RemoveRoute(ctx, caddysvc.RouteID(ct.Name))
		}
		if err := h.runtime.DeleteContainer(ctx, ct.ID, true); err != nil {
			h.log.Audit(
				ctx, logger.AuditEntry{
					Action: logger.AuditContainerDelete, ResourceID: input.StackID, Success: false,
					Details: fmt.Sprintf("stack delete failed on container %q err=%s", ct.Name, err),
				},
			)
			return nil, fmt.Errorf("delete container %q: %w", ct.Name, err)
		}
		deleted = append(deleted, ct.Name)
	}
	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditContainerDelete, ResourceID: input.StackID, Success: true,
			Details: fmt.Sprintf("stack delete: %d container(s) [%s]", len(deleted), strings.Join(deleted, ", ")),
		},
	)
	return nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseComposeEnv(raw any) []string {
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
		for key, val := range v {
			if val == nil {
				result = append(result, key)
			} else {
				result = append(result, fmt.Sprintf("%s=%v", key, val))
			}
		}
	}
	return result
}

func parseComposePorts(ports []string) []runtime.PortBinding {
	var result []runtime.PortBinding
	for _, p := range ports {
		parts := strings.Split(p, ":")
		if len(parts) < 2 {
			continue
		}
		hostPort := parts[len(parts)-2]
		containerPart := parts[len(parts)-1]
		protocol := "tcp"
		if strings.Contains(containerPart, "/") {
			pp := strings.SplitN(containerPart, "/", 2)
			containerPart = pp[0]
			protocol = pp[1]
		}
		containerPort, err := strconv.Atoi(containerPart)
		if err != nil {
			continue
		}
		result = append(
			result, runtime.PortBinding{
				HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol,
			},
		)
	}
	return result
}

func parseComposeVolumes(volumes []string, stackName string) []runtime.VolumeMount {
	var result []runtime.VolumeMount
	for _, v := range volumes {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name, mount := parts[0], parts[1]
		if !strings.HasPrefix(name, "/") {
			name = fmt.Sprintf("%s_%s", stackName, name)
		}
		result = append(result, runtime.VolumeMount{Name: name, Mount: mount})
	}
	return result
}
