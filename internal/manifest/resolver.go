package manifest

import (
	"fmt"
	"strings"
)

var defaults = struct {
	Namespace    string
	TLS          bool
	Interval     string
	Timeout      string
	Retries      int
	StartupGrace string
	Restart      string
	RestartDelay string
	Strategy     string
	Replicas     int
	AutoMetric   string
	AutoTarget   int
	AutoMin      int
	AutoMax      int
	FallbackPort int
}{
	Namespace:    "default",
	TLS:          true,
	Interval:     "10s",
	Timeout:      "3s",
	Retries:      3,
	StartupGrace: "10s",
	Restart:      "always",
	RestartDelay: "5s",
	Strategy:     "rolling",
	Replicas:     1,
	AutoMetric:   "cpu",
	AutoTarget:   70,
	AutoMin:      1,
	AutoMax:      5,
	FallbackPort: 8080,
}

var knownPorts = map[string]int{
	"traefik/whoami":  80,
	"nginx":           80,
	"httpd":           80,
	"caddy":           80,
	"postgres":        5432,
	"redis":           6379,
	"mysql":           3306,
	"mariadb":         3306,
	"mongo":           27017,
	"grafana/grafana": 3000,
	"minio/minio":     9000,
	"rabbitmq":        5672,
}

type ResolvedManifest struct {
	Name      string
	Namespace string
	Labels    map[string]string

	Container   ResolvedContainer
	Expose      ResolvedExpose
	Health      ResolvedHealth
	Scaling     ResolvedScaling
	DependsOn   []string
	Alerts      ResolvedAlerts
	Maintenance ResolvedMaintenance

	Build *ResolvedBuild
}

type ResolvedBuild struct {
	Tag              string
	IsGit            bool
	GitURL           string
	Branch           string
	DockerfilePath   string
	DockerfileInline string
	BuildArgs        map[string]string
	Target           string
}

type ResolvedContainer struct {
	Image     string
	Env       []EnvVar
	Volumes   []VolumeSpec
	Resources *ResourceSpec
}

type ResolvedExpose struct {
	Port   int
	Domain string
	TLS    bool
	WWW    bool
}

type ResolvedHealth struct {
	// Test is a raw Docker healthcheck command — takes priority over HTTP/TCP.
	Test         []string
	HTTP         string
	TCP          bool
	Interval     string
	Timeout      string
	Retries      int
	StartupGrace string
}

type ResolvedScaling struct {
	Replicas     int
	Restart      string
	RestartDelay string
	MaxRestarts  int
	Strategy     string
	Autoscaling  ResolvedAutoscaling
}

type ResolvedAutoscaling struct {
	Enabled bool
	Metric  string
	Target  int
	Min     int
	Max     int
}

type ResolvedAlerts struct {
	OnCrash  bool
	OnDeploy bool
	Webhook  string
}

type ResolvedMaintenance struct {
	RestartCron string
}

func Resolve(m *ServiceManifest) (*ResolvedManifest, error) {
	if err := validate(m); err != nil {
		return nil, err
	}

	r := &ResolvedManifest{}

	r.Name = m.Metadata.Name
	r.Namespace = or(m.Metadata.Namespace, defaults.Namespace)
	r.Labels = m.Metadata.Labels
	if r.Labels == nil {
		r.Labels = map[string]string{}
	}
	r.Labels["tidefly.service"] = r.Name

	image := m.Spec.Container.Image
	if b := m.Spec.Container.Build; b != nil {
		resolved, err := resolveBuild(r.Name, b)
		if err != nil {
			return nil, err
		}
		r.Build = resolved
		image = resolved.Tag
	}

	r.Container = ResolvedContainer{
		Image:     image,
		Env:       m.Spec.Container.Env,
		Volumes:   m.Spec.Container.Volumes,
		Resources: m.Spec.Container.Resources,
	}

	expose := ResolvedExpose{TLS: defaults.TLS}
	if m.Spec.Expose != nil {
		expose.Port = m.Spec.Expose.Port
		expose.Domain = m.Spec.Expose.Domain
		expose.TLS = m.Spec.Expose.TLS
		expose.WWW = m.Spec.Expose.WWW
	}
	if expose.Port == 0 {
		expose.Port = detectPort(image)
	}
	r.Expose = expose

	health := ResolvedHealth{
		Interval:     defaults.Interval,
		Timeout:      defaults.Timeout,
		Retries:      defaults.Retries,
		StartupGrace: defaults.StartupGrace,
	}
	if m.Spec.Health != nil {
		// Raw test command takes priority
		health.Test = m.Spec.Health.Test
		health.HTTP = m.Spec.Health.HTTP
		health.TCP = m.Spec.Health.TCP
		health.Interval = or(m.Spec.Health.Interval, defaults.Interval)
		health.Timeout = or(m.Spec.Health.Timeout, defaults.Timeout)
		// StartPeriod is an alias for StartupGrace (Docker Compose compat)
		startupGrace := or(m.Spec.Health.StartupGrace, m.Spec.Health.StartPeriod)
		health.StartupGrace = or(startupGrace, defaults.StartupGrace)
		if m.Spec.Health.Retries > 0 {
			health.Retries = m.Spec.Health.Retries
		}
	}
	// Only fall back to TCP if no explicit test or HTTP is set
	if len(health.Test) == 0 && health.HTTP == "" {
		health.TCP = true
	}
	r.Health = health

	scaling := ResolvedScaling{
		Replicas:     defaults.Replicas,
		Restart:      defaults.Restart,
		RestartDelay: defaults.RestartDelay,
		Strategy:     defaults.Strategy,
		Autoscaling: ResolvedAutoscaling{
			Metric: defaults.AutoMetric,
			Target: defaults.AutoTarget,
			Min:    defaults.AutoMin,
			Max:    defaults.AutoMax,
		},
	}
	if m.Spec.Scaling != nil {
		if m.Spec.Scaling.Replicas > 0 {
			scaling.Replicas = m.Spec.Scaling.Replicas
		}
		scaling.Restart = or(m.Spec.Scaling.Restart, defaults.Restart)
		scaling.RestartDelay = or(m.Spec.Scaling.RestartDelay, defaults.RestartDelay)
		scaling.MaxRestarts = m.Spec.Scaling.MaxRestarts
		scaling.Strategy = or(m.Spec.Scaling.Strategy, defaults.Strategy)
		if as := m.Spec.Scaling.Autoscaling; as != nil {
			scaling.Autoscaling.Enabled = as.Enabled
			scaling.Autoscaling.Metric = or(as.Metric, defaults.AutoMetric)
			if as.Target > 0 {
				scaling.Autoscaling.Target = as.Target
			}
			if as.Min > 0 {
				scaling.Autoscaling.Min = as.Min
			}
			if as.Max > 0 {
				scaling.Autoscaling.Max = as.Max
			}
		}
	}
	r.Scaling = scaling

	r.DependsOn = m.Spec.DependsOn

	alerts := ResolvedAlerts{OnCrash: true}
	if m.Spec.Alerts != nil {
		alerts.OnCrash = m.Spec.Alerts.OnCrash
		alerts.OnDeploy = m.Spec.Alerts.OnDeploy
		alerts.Webhook = m.Spec.Alerts.Webhook
	}
	r.Alerts = alerts

	if m.Spec.Maintenance != nil {
		r.Maintenance.RestartCron = m.Spec.Maintenance.RestartCron
	}

	return r, nil
}

func resolveBuild(serviceName string, b *BuildSpec) (*ResolvedBuild, error) {
	tag := fmt.Sprintf("localhost/tidefly/%s:latest", serviceName)

	rb := &ResolvedBuild{
		Tag:              tag,
		DockerfilePath:   or(b.Dockerfile, "Dockerfile"),
		DockerfileInline: b.DockerfileInline,
		BuildArgs:        b.BuildArgs,
		Target:           b.Target,
	}

	if b.IsGitContext() {
		gitURL, branch := b.GitURL()
		rb.IsGit = true
		rb.GitURL = gitURL
		rb.Branch = branch
	}

	return rb, nil
}

func validate(m *ServiceManifest) error {
	if m.APIVersion != "tidefly/v1" {
		return fmt.Errorf("manifest: unsupported apiVersion %q — expected \"tidefly/v1\"", m.APIVersion)
	}
	if m.Kind != "service" {
		return fmt.Errorf("manifest: unsupported kind %q — expected \"service\"", m.Kind)
	}
	if m.Metadata.Name == "" {
		return fmt.Errorf("manifest: metadata.name is required")
	}

	hasImage := m.Spec.Container.Image != ""
	hasBuild := m.Spec.Container.Build != nil
	if !hasImage && !hasBuild {
		return fmt.Errorf("manifest: spec.container.image or spec.container.build is required")
	}
	if hasImage && hasBuild {
		return fmt.Errorf("manifest: spec.container.image and spec.container.build are mutually exclusive")
	}

	if hasBuild {
		b := m.Spec.Container.Build
		hasInline := b.DockerfileInline != ""
		hasContext := b.Context != ""
		if !hasInline && !hasContext {
			return fmt.Errorf("manifest: spec.container.build requires context or dockerfileInline")
		}
		if b.IsGitContext() {
			gitURL, _ := b.GitURL()
			if gitURL == "" {
				return fmt.Errorf("manifest: spec.container.build.context git URL is invalid")
			}
		}
	}

	if s := m.Spec.Scaling; s != nil {
		allowed := map[string]bool{"always": true, "on-failure": true, "never": true, "": true}
		if !allowed[s.Restart] {
			return fmt.Errorf("manifest: invalid restart policy %q", s.Restart)
		}
		allowedStrategy := map[string]bool{"rolling": true, "recreate": true, "blue-green": true, "": true}
		if !allowedStrategy[s.Strategy] {
			return fmt.Errorf("manifest: invalid strategy %q — must be one of: rolling, recreate, blue-green", s.Strategy)
		}
		if as := s.Autoscaling; as != nil && as.Enabled {
			allowedMetric := map[string]bool{"cpu": true, "memory": true, "requests": true}
			if !allowedMetric[as.Metric] {
				return fmt.Errorf("manifest: invalid autoscaling metric %q", as.Metric)
			}
			if as.Min > as.Max {
				return fmt.Errorf("manifest: autoscaling min (%d) must be <= max (%d)", as.Min, as.Max)
			}
		}
	}

	for i, env := range m.Spec.Container.Env {
		if env.Value != "" && env.Secret != "" {
			return fmt.Errorf("manifest: env[%d] %q — value and secret are mutually exclusive", i, env.Name)
		}
	}

	return nil
}

func detectPort(image string) int {
	base := strings.SplitN(image, ":", 2)[0]
	for prefix, port := range knownPorts {
		if base == prefix || strings.HasPrefix(base, prefix+"/") || strings.HasSuffix(base, "/"+prefix) {
			return port
		}
	}
	return defaults.FallbackPort
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
