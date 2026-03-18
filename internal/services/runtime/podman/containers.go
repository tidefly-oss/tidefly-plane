package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

func (p *Runtime) ListContainers(ctx context.Context, all bool) ([]runtime.Container, error) {
	q := url.Values{}
	q.Set("all", fmt.Sprintf("%v", all))

	var raw []struct {
		ID       *string           `json:"Id"`
		Names    []string          `json:"Names"`
		Image    *string           `json:"Image"`
		State    *string           `json:"State"`
		Created  json.RawMessage   `json:"Created"`
		Labels   map[string]string `json:"Labels"`
		Networks []string          `json:"Networks"`
		Ports    []struct {
			HostIP        *string `json:"host_ip"`
			HostPort      *int    `json:"host_port"`
			ContainerPort *int    `json:"container_port"`
			Protocol      *string `json:"protocol"`
		} `json:"Ports"`
	}

	_, err := p.c.getJSON(ctx, "/libpod/containers/json", q, &raw)
	if err != nil {
		return nil, fmt.Errorf("podman list containers: %w", err)
	}

	result := make([]runtime.Container, 0, len(raw))
	for _, c := range raw {
		labels := c.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		if labels["tidefly.internal"] == "true" {
			continue
		}

		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		id := derefStr(c.ID)
		if len(id) > 12 {
			id = id[:12]
		}

		state := derefStr(c.State)

		var ports []runtime.Port
		for _, p := range c.Ports {
			port := runtime.Port{Protocol: "tcp"}
			if p.HostIP != nil {
				port.HostIP = *p.HostIP
			}
			if p.HostPort != nil {
				port.HostPort = uint16(*p.HostPort)
			}
			if p.ContainerPort != nil {
				port.ContainerPort = uint16(*p.ContainerPort)
			}
			if p.Protocol != nil {
				port.Protocol = *p.Protocol
			}
			ports = append(ports, port)
		}

		created := parseCreated(c.Created)

		result = append(
			result, runtime.Container{
				ID:       id,
				Name:     name,
				Image:    derefStr(c.Image),
				Status:   runtime.MapStatus(state),
				State:    state,
				Created:  created,
				Ports:    ports,
				Labels:   labels,
				Networks: c.Networks,
			},
		)
	}
	return result, nil
}

// parseCreated handles Created as RFC3339 string or Unix int64.
func parseCreated(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	var ts int64
	if err := json.Unmarshal(raw, &ts); err == nil {
		return time.Unix(ts, 0)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (p *Runtime) GetContainer(ctx context.Context, id string) (*runtime.ContainerDetails, error) {
	var inspect struct {
		ID    *string `json:"Id"`
		Name  *string `json:"Name"`
		Path  *string `json:"Path"`
		State *struct {
			Status  *string `json:"Status"`
			Running *bool   `json:"Running"`
		} `json:"State"`
		Created   *string `json:"Created"`
		ImageName *string `json:"ImageName"`
		Config    *struct {
			Labels     map[string]string `json:"Labels"`
			Env        []string          `json:"Env"`
			Entrypoint json.RawMessage   `json:"Entrypoint"` // string or []string depending on Podman version
		} `json:"Config"`
		HostConfig *struct {
			RestartPolicy *struct {
				Name              *string `json:"Name"`
				MaximumRetryCount *int    `json:"MaximumRetryCount"`
			} `json:"RestartPolicy"`
			NanoCpus   *int64  `json:"NanoCpus"`
			CpuQuota   *int64  `json:"CpuQuota"`
			CpuPeriod  *uint64 `json:"CpuPeriod"`
			Memory     *int64  `json:"Memory"`
			MemorySwap *int64  `json:"MemorySwap"`
		} `json:"HostConfig"`
		Mounts []struct {
			Source      *string `json:"Source"`
			Destination *string `json:"Destination"`
			Mode        *string `json:"Mode"`
			RW          *bool   `json:"RW"`
		} `json:"Mounts"`
		NetworkSettings *struct {
			Networks map[string]struct{} `json:"Networks"`
			Ports    map[string][]struct {
				HostIP   *string `json:"HostIp"`
				HostPort *string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}

	_, err := p.c.getJSON(ctx, "/libpod/containers/"+escPath(id)+"/json", nil, &inspect)
	if err != nil {
		return nil, fmt.Errorf("podman inspect container: %w", err)
	}

	shortID := derefStr(inspect.ID)
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	name := strings.TrimPrefix(derefStr(inspect.Name), "/")

	state := ""
	if inspect.State != nil && inspect.State.Status != nil {
		state = *inspect.State.Status
	}

	var labels map[string]string
	var envVars, entrypoint []string
	if inspect.Config != nil {
		labels = inspect.Config.Labels
		envVars = inspect.Config.Env
		entrypoint = parseEntrypoint(inspect.Config.Entrypoint)
	}
	if labels == nil {
		labels = map[string]string{}
	}

	restartPolicy := ""
	if inspect.HostConfig != nil && inspect.HostConfig.RestartPolicy != nil {
		restartPolicy = derefStr(inspect.HostConfig.RestartPolicy.Name)
	}

	var mounts []runtime.Mount
	for _, m := range inspect.Mounts {
		mounts = append(
			mounts, runtime.Mount{
				Source:      derefStr(m.Source),
				Destination: derefStr(m.Destination),
				Mode:        derefStr(m.Mode),
				RW:          m.RW != nil && *m.RW,
			},
		)
	}

	var networks []string
	var ports []runtime.Port
	if inspect.NetworkSettings != nil {
		for netName := range inspect.NetworkSettings.Networks {
			networks = append(networks, netName)
		}
		for cpStr, bindings := range inspect.NetworkSettings.Ports {
			if bindings == nil {
				continue
			}
			proto := "tcp"
			portStr := cpStr
			if parts := strings.SplitN(cpStr, "/", 2); len(parts) == 2 {
				portStr, proto = parts[0], parts[1]
			}
			cp, _ := strconv.ParseUint(portStr, 10, 16)
			for _, b := range bindings {
				hp, _ := strconv.ParseUint(derefStr(b.HostPort), 10, 16)
				ports = append(
					ports, runtime.Port{
						HostIP:        derefStr(b.HostIP),
						HostPort:      uint16(hp),
						ContainerPort: uint16(cp),
						Protocol:      proto,
					},
				)
			}
		}
	}

	return &runtime.ContainerDetails{
		Container: runtime.Container{
			ID:     shortID,
			Name:   name,
			Image:  derefStr(inspect.ImageName),
			Status: runtime.MapStatus(state),
			State:  state,
			Labels: labels,
			Mounts: mounts,
			Ports:  ports,
		},
		Command:       derefStr(inspect.Path),
		Entrypoint:    entrypoint,
		Env:           envVars,
		Mounts:        mounts,
		Networks:      networks,
		RestartPolicy: restartPolicy,
	}, nil
}

func (p *Runtime) CreateContainer(ctx context.Context, spec runtime.ContainerSpec) error {
	// Podman requires fully-qualified image names (e.g. docker.io/nginx:alpine)
	img := qualifyImage(spec.Image, true)

	// Auto-pull image if not present locally
	checkCode, _ := p.c.getJSON(ctx, "/libpod/images/"+escPath(img)+"/json", nil, nil)
	if checkCode != 200 {
		q := url.Values{}
		q.Set("reference", img)
		pullCode, pullBody, pullErr := p.c.post(ctx, "/libpod/images/pull", q, nil)
		if pullErr != nil {
			return fmt.Errorf("podman pull image %q: %w", img, pullErr)
		}
		if pullCode != 200 {
			return fmt.Errorf(
				"podman pull image %q: %w", img, newAPIError(pullCode, "POST", "/libpod/images/pull", pullBody),
			)
		}
	}

	// Remove existing container with same name — ignore errors (404 = doesn't exist, that's fine)
	_, _ = p.c.delete(ctx, "/libpod/containers/"+escPath(spec.Name), url.Values{"force": []string{"true"}})

	// Port mappings
	type portMapping struct {
		HostIP        string `json:"host_ip,omitempty"`
		HostPort      uint16 `json:"host_port"`
		ContainerPort uint16 `json:"container_port"`
		Protocol      string `json:"protocol"`
	}
	var portMappings []portMapping
	for _, pm := range spec.Ports {
		proto := pm.Protocol
		if proto == "" {
			proto = "tcp"
		}
		hp, _ := strconv.ParseUint(pm.HostPort, 10, 16)
		portMappings = append(
			portMappings, portMapping{
				HostIP:        "0.0.0.0",
				HostPort:      uint16(hp),
				ContainerPort: uint16(pm.ContainerPort),
				Protocol:      proto,
			},
		)
	}

	// Volume mounts
	type namedVolume struct {
		Name    string   `json:"name"`
		Dest    string   `json:"dest"`
		Options []string `json:"options,omitempty"`
	}
	var namedVolumes []namedVolume
	for _, v := range spec.Volumes {
		namedVolumes = append(namedVolumes, namedVolume{Name: v.Name, Dest: v.Mount})
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

	// Healthcheck
	type healthConfig struct {
		Test        []string      `json:"test,omitempty"`
		Interval    time.Duration `json:"interval,omitempty"`
		Timeout     time.Duration `json:"timeout,omitempty"`
		Retries     int           `json:"retries,omitempty"`
		StartPeriod time.Duration `json:"start_period,omitempty"`
	}
	var hc *healthConfig
	if spec.Healthcheck != nil {
		hc = &healthConfig{
			Test:        spec.Healthcheck.Test,
			Interval:    spec.Healthcheck.Interval,
			Timeout:     spec.Healthcheck.Timeout,
			Retries:     spec.Healthcheck.Retries,
			StartPeriod: spec.Healthcheck.StartPeriod,
		}
	}

	body := map[string]any{
		"image":          img,
		"name":           spec.Name,
		"env":            envSliceToMap(spec.Env),
		"labels":         spec.Labels,
		"portmappings":   portMappings,
		"named_volumes":  namedVolumes,
		"restart_policy": restart,
		"netns":          map[string]any{"nsmode": "bridge"},
		"networks":       map[string]any{spec.Network: map[string]any{}},
	}
	if len(cmd) > 0 {
		body["command"] = cmd
	}
	if hc != nil {
		body["healthconfig"] = hc
	}

	// Podman returns {"Id": "..."} on 201
	_, createBody, err := p.c.post(ctx, "/libpod/containers/create", nil, body)
	if err != nil {
		return fmt.Errorf("podman create container %q: %w", spec.Name, err)
	}

	var createResp struct {
		ID string `json:"Id"`
	}
	if jsonErr := json.Unmarshal(createBody, &createResp); jsonErr != nil || createResp.ID == "" {
		return fmt.Errorf(
			"podman create container %q: %w", spec.Name,
			newAPIError(0, "POST", "/libpod/containers/create", createBody),
		)
	}

	// Start using the ID from the create response — no extra inspect needed
	_, startBody, err := p.c.post(ctx, "/libpod/containers/"+escPath(createResp.ID)+"/start", nil, nil)
	if err != nil {
		return fmt.Errorf("podman start container %q: %w", spec.Name, err)
	}
	// 204 = started, 304 = already running — both are fine
	// Any error response will have a non-empty body with a message field
	if len(startBody) > 0 {
		var startErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(startBody, &startErr) == nil && startErr.Message != "" {
			return fmt.Errorf("podman start container %q: %s", spec.Name, startErr.Message)
		}
	}

	return nil
}

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func (p *Runtime) StartContainer(ctx context.Context, id string) error {
	_, b, err := p.c.post(ctx, "/libpod/containers/"+escPath(id)+"/start", nil, nil)
	if err != nil {
		return fmt.Errorf("podman start %q: %w", id, err)
	}
	if len(b) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(b, &e) == nil && e.Message != "" {
			return fmt.Errorf("podman start %q: %s", id, e.Message)
		}
	}
	return nil
}

func (p *Runtime) StopContainer(ctx context.Context, id string, opts runtime.StopOptions) error {
	q := url.Values{}
	if opts.Timeout != nil {
		q.Set("t", fmt.Sprintf("%d", *opts.Timeout))
	}
	_, b, err := p.c.post(ctx, "/libpod/containers/"+escPath(id)+"/stop", q, nil)
	if err != nil {
		return fmt.Errorf("podman stop %q: %w", id, err)
	}
	if len(b) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(b, &e) == nil && e.Message != "" {
			return fmt.Errorf("podman stop %q: %s", id, e.Message)
		}
	}
	return nil
}

func (p *Runtime) RestartContainer(ctx context.Context, id string, opts runtime.StopOptions) error {
	q := url.Values{}
	if opts.Timeout != nil {
		q.Set("t", fmt.Sprintf("%d", *opts.Timeout))
	}
	_, b, err := p.c.post(ctx, "/libpod/containers/"+escPath(id)+"/restart", q, nil)
	if err != nil {
		return fmt.Errorf("podman restart %q: %w", id, err)
	}
	if len(b) > 0 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(b, &e) == nil && e.Message != "" {
			return fmt.Errorf("podman restart %q: %s", id, e.Message)
		}
	}
	return nil
}

func (p *Runtime) DeleteContainer(ctx context.Context, id string, force bool) error {
	q := url.Values{}
	q.Set("force", fmt.Sprintf("%v", force))
	_, err := p.c.delete(ctx, "/libpod/containers/"+escPath(id), q)
	if err != nil {
		return fmt.Errorf("podman delete %q: %w", id, err)
	}
	return nil
}

func (p *Runtime) ContainerLogs(ctx context.Context, id string, opts runtime.LogOptions) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("follow", fmt.Sprintf("%v", opts.Follow))
	q.Set("stdout", "true")
	q.Set("stderr", "true")
	q.Set("timestamps", fmt.Sprintf("%v", opts.Timestamps))
	if opts.Tail != "" {
		q.Set("tail", opts.Tail)
	}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}

	resp, err := p.c.get(ctx, "/libpod/containers/"+escPath(id)+"/logs", q)
	if err != nil {
		return nil, fmt.Errorf("podman logs %q: %w", id, err)
	}
	return resp.Body, nil
}

func (p *Runtime) isRunning(ctx context.Context, containerID string) (bool, error) {
	var inspect struct {
		State *struct {
			Running *bool `json:"Running"`
		} `json:"State"`
	}
	_, err := p.c.getJSON(ctx, "/libpod/containers/"+escPath(containerID)+"/json", nil, &inspect)
	if err != nil {
		return false, err
	}
	if inspect.State != nil && inspect.State.Running != nil {
		return *inspect.State.Running, nil
	}
	return false, nil
}

// parseEntrypoint handles Podman returning Entrypoint as either a JSON string
// or a JSON array depending on how the container was created.
//
//	"/bin/sh"        → ["/bin/sh"]
//	["/bin/sh", "-c"] → ["/bin/sh", "-c"]
//	null / ""        → nil
func parseEntrypoint(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	// Try array first (most common)
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	// Fall back to plain string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return []string{s}
	}
	return nil
}
