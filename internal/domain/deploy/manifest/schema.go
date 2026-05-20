package manifest

// ── Service Manifest ──────────────────────────────────────────────────────────

// ServiceManifest is the top-level Tidefly service definition.
// Supports three container sources:
//
//  1. Pre-built image:   spec.container.image = "nginx:alpine"
//  2. Dockerfile build:  spec.container.build.dockerfile = "Dockerfile"
//  3. Git build:         spec.container.build.context = "git://github.com/org/repo#main"
//
// Minimal image example:
//
//	{ "apiVersion": "tidefly/v1", "kind": "service",
//	  "metadata": { "name": "whoami" },
//	  "spec": { "container": { "image": "traefik/whoami" }, "expose": { "domain": "whoami.example.com" } } }
//
// Minimal dockerfile example:
//
//	{ "apiVersion": "tidefly/v1", "kind": "service",
//	  "metadata": { "name": "myapp" },
//	  "spec": { "container": { "build": { "dockerfile": "Dockerfile" } }, "expose": { "domain": "myapp.example.com" } } }
type ServiceManifest struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Metadata   Metadata    `json:"metadata"`
	Spec       ServiceSpec `json:"spec"`
}

// ── Stack Manifest ────────────────────────────────────────────────────────────

// StackManifest is the top-level definition for a Docker Compose-based stack.
// Each service in the compose file becomes a separate Tidefly service.
//
// Example:
//
//	{ "apiVersion": "tidefly/v1", "kind": "stack",
//	  "metadata": { "name": "mystack" },
//	  "spec": { "compose": "<inline docker-compose.yml content>" } }
type StackManifest struct {
	APIVersion string    `json:"apiVersion"`
	Kind       string    `json:"kind"` // "stack"
	Metadata   Metadata  `json:"metadata"`
	Spec       StackSpec `json:"spec"`
}

type StackSpec struct {
	// Compose is the inline docker-compose.yml content.
	Compose string `json:"compose"`

	// Domain is the public hostname for the primary service.
	Domain string `json:"domain,omitempty"`

	// Expose controls whether the primary service is exposed via Caddy.
	Expose bool `json:"expose,omitempty"`
}

// ── Shared Metadata ───────────────────────────────────────────────────────────

type Metadata struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// ── Service Spec ──────────────────────────────────────────────────────────────

type ServiceSpec struct {
	Container   ContainerSpec    `json:"container"`
	Expose      *ExposeSpec      `json:"expose,omitempty"`
	Health      *HealthSpec      `json:"health,omitempty"`
	Scaling     *ScalingSpec     `json:"scaling,omitempty"`
	DependsOn   []string         `json:"dependsOn,omitempty"`
	Alerts      *AlertSpec       `json:"alerts,omitempty"`
	Maintenance *MaintenanceSpec `json:"maintenance,omitempty"`
}

// ContainerSpec describes the container to run.
// Either Image or Build must be set — not both.
type ContainerSpec struct {
	// Image is the OCI image reference, e.g. "ghcr.io/myorg/api:latest".
	// Required when Build is not set.
	Image string `json:"image,omitempty"`

	// Build describes how to build the image from source.
	// When set, Image is derived from the build tag and does not need to be specified.
	Build *BuildSpec `json:"build,omitempty"`

	// Env is the list of environment variables.
	Env []EnvVar `json:"env,omitempty"`

	// Volumes is the list of named volumes to mount.
	Volumes []VolumeSpec `json:"volumes,omitempty"`

	// Resources defines CPU/memory limits.
	Resources *ResourceSpec `json:"resources,omitempty"`
}

// BuildSpec describes how to build a container image from source.
type BuildSpec struct {
	// Context is the build context path or a git URL.
	//   - Local:  "." | "./subdir"
	//   - Git:    "git://github.com/org/repo#branch"
	// Defaults to "." (current directory, requires dockerfile content or path).
	Context string `json:"context,omitempty"`

	// Dockerfile is the path to the Dockerfile within the context. Default: "Dockerfile".
	Dockerfile string `json:"dockerfile,omitempty"`

	// DockerfileInline is the raw Dockerfile content.
	// Use this to embed a Dockerfile directly in the manifest without a build context.
	DockerfileInline string `json:"dockerfileInline,omitempty"`

	// Target is the multi-stage build target to build. Optional.
	Target string `json:"target,omitempty"`

	// Args are build-time variables passed to docker build --build-arg.
	BuildArgs map[string]string `json:"args,omitempty"`
}

// IsGitContext returns true if the build context is a git URL.
func (b *BuildSpec) IsGitContext() bool {
	return len(b.Context) > 6 && b.Context[:6] == "git://"
}

// GitURL returns the git URL and branch from a git:// context.
// "git://github.com/org/repo#main" → ("https://github.com/org/repo", "main")
func (b *BuildSpec) GitURL() (url, branch string) {
	raw := b.Context[6:] // strip "git://"
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

// ── Supporting types ──────────────────────────────────────────────────────────

type EnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value,omitempty"`
	Secret string `json:"secret,omitempty"` // "secret-name/key"
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

type HealthSpec struct {
	HTTP         string `json:"http,omitempty"`
	TCP          bool   `json:"tcp,omitempty"`
	Interval     string `json:"interval,omitempty"`
	Timeout      string `json:"timeout,omitempty"`
	Retries      int    `json:"retries,omitempty"`
	StartupGrace string `json:"startupGrace,omitempty"`
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
