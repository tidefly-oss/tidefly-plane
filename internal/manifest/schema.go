package manifest

// ── Service Manifest ──────────────────────────────────────────────────────────

// ServiceManifest is the top-level Tidefly service definition.
type ServiceManifest struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   Metadata    `json:"metadata"`
	Spec       ServiceSpec `json:"spec"`
}

// ── Stack Manifest ────────────────────────────────────────────────────────────

type StackManifest struct {
	APIVersion string    `json:"apiVersion"`
	Kind       string    `json:"kind"`
	Metadata   Metadata  `json:"metadata"`
	Spec       StackSpec `json:"spec"`
}

type StackSpec struct {
	Compose string `json:"compose"`
	Domain  string `json:"domain,omitempty"`
	Expose  bool   `json:"expose,omitempty"`
}

type Metadata struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type ServiceSpec struct {
	Container   ContainerSpec    `json:"container"`
	Expose      *ExposeSpec      `json:"expose,omitempty"`
	Health      *HealthSpec      `json:"health,omitempty"`
	Scaling     *ScalingSpec     `json:"scaling,omitempty"`
	DependsOn   []string         `json:"dependsOn,omitempty"`
	Alerts      *AlertSpec       `json:"alerts,omitempty"`
	Maintenance *MaintenanceSpec `json:"maintenance,omitempty"`
}

type ContainerSpec struct {
	Image     string        `json:"image,omitempty"`
	Build     *BuildSpec    `json:"build,omitempty"`
	Env       []EnvVar      `json:"env,omitempty"`
	Volumes   []VolumeSpec  `json:"volumes,omitempty"`
	Resources *ResourceSpec `json:"resources,omitempty"`
}

type BuildSpec struct {
	Context          string            `json:"context,omitempty"`
	Dockerfile       string            `json:"dockerfile,omitempty"`
	DockerfileInline string            `json:"dockerfileInline,omitempty"`
	Target           string            `json:"target,omitempty"`
	BuildArgs        map[string]string `json:"args,omitempty"`
}

func (b *BuildSpec) IsGitContext() bool {
	return len(b.Context) > 6 && b.Context[:6] == "git://"
}

func (b *BuildSpec) GitURL() (url, branch string) {
	raw := b.Context[6:]
	parts := splitLast(raw, "#")
	return "https://" + parts[0], parts[1]
}

func splitLast(s, sep string) [2]string {
	idx := len(s)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep[0] {
			idx = i
			break
		}
	}
	if idx == len(s) {
		return [2]string{s, "main"}
	}
	return [2]string{s[:idx], s[idx+1:]}
}

type EnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Secret string `json:"secret,omitempty"`
}

type VolumeSpec struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
}

type ResourceSpec struct {
	Memory      string `json:"memory,omitempty"`
	CPU         string `json:"cpu,omitempty"`
	MemoryLimit string `json:"memoryLimit,omitempty"`
	CPULimit    string `json:"cpuLimit,omitempty"`
}

type ExposeSpec struct {
	Port   int    `json:"port,omitempty"`
	Domain string `json:"domain,omitempty"`
	TLS    bool   `json:"tls,omitempty"`
	WWW    bool   `json:"www,omitempty"`
}

// HealthSpec defines how a service's health is checked.
// Either Test (raw Docker healthcheck command) or HTTP/TCP must be set.
// If neither is set, a TCP check on the exposed port is used as default.
type HealthSpec struct {
	// Test is a raw Docker-style healthcheck command, e.g. ["CMD", "wget", "-qO-", "http://localhost/"].
	// When set, HTTP and TCP are ignored.
	Test []string `json:"test,omitempty"`

	// HTTP is the path to check via HTTP GET, e.g. "/health".
	HTTP string `json:"http,omitempty"`

	// TCP performs a TCP connection check on the exposed port.
	TCP bool `json:"tcp,omitempty"`

	Interval     string `json:"interval,omitempty"`
	Timeout      string `json:"timeout,omitempty"`
	Retries      int    `json:"retries,omitempty"`
	StartupGrace string `json:"startupGrace,omitempty"`

	// StartPeriod is an alias for StartupGrace for Docker Compose compatibility.
	StartPeriod string `json:"startPeriod,omitempty"`
}

type ScalingSpec struct {
	Replicas     int              `json:"replicas,omitempty"`
	Restart      string           `json:"restart,omitempty"`
	RestartDelay string           `json:"restartDelay,omitempty"`
	MaxRestarts  int              `json:"maxRestarts,omitempty"`
	Strategy     string           `json:"strategy,omitempty"`
	Autoscaling  *AutoscalingSpec `json:"autoscaling,omitempty"`
}

type AutoscalingSpec struct {
	Enabled bool   `json:"enabled"`
	Metric  string `json:"metric,omitempty"`
	Target  int    `json:"target,omitempty"`
	Min     int    `json:"min,omitempty"`
	Max     int    `json:"max,omitempty"`
}

type AlertSpec struct {
	OnCrash  bool   `json:"onCrash,omitempty"`
	OnDeploy bool   `json:"onDeploy,omitempty"`
	Webhook  string `json:"webhook,omitempty"`
}

type MaintenanceSpec struct {
	RestartCron string `json:"restartCron,omitempty"`
}
