// Package manifest defines the Tidefly service manifest schema, resolver, and adapter.
package manifest

import (
	"strings"
	"time"

	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
)

// ToContainerSpec converts a ResolvedManifest into a runtime.ContainerSpec.
func ToContainerSpec(r *ResolvedManifest, networkName string, isPodman bool) runtime.ContainerSpec {
	image := qualifyImageManifest(r.Container.Image, isPodman)

	env := make([]string, 0, len(r.Container.Env))
	for _, e := range r.Container.Env {
		env = append(env, e.Name+"="+e.Value)
	}

	volumes := make([]runtime.VolumeMount, 0, len(r.Container.Volumes))
	for _, v := range r.Container.Volumes {
		volumes = append(volumes, runtime.VolumeMount{Name: v.Name, Mount: v.MountPath})
	}

	hc := toHealthcheck(r.Health, r.Expose.Port)

	labels := make(map[string]string, len(r.Labels))
	for k, v := range r.Labels {
		labels[k] = v
	}
	if r.Expose.Domain != "" {
		labels["tidefly.domain"] = r.Expose.Domain
	}
	labels["tidefly.port"] = itoa(r.Expose.Port)

	if r.Build != nil {
		if r.Build.IsGit {
			labels["tidefly.source"] = "git"
			labels["tidefly.git-url"] = r.Build.GitURL
			labels["tidefly.git-branch"] = r.Build.Branch
			labels["tidefly.dockerfile-path"] = r.Build.DockerfilePath
		} else if r.Build.DockerfileInline != "" {
			labels["tidefly.source"] = "dockerfile"
		}
		labels["tidefly.build-tag"] = r.Build.Tag
	}

	restart := mapRestartPolicy(r.Scaling.Restart)

	return runtime.ContainerSpec{
		Name:        r.Name,
		Image:       image,
		Env:         env,
		Ports:       nil,
		Volumes:     volumes,
		Labels:      labels,
		Healthcheck: hc,
		Restart:     restart,
		Network:     networkName,
	}
}

func toHealthcheck(h ResolvedHealth, port int) *runtime.Healthcheck {
	interval := parseDurationManifest(h.Interval)
	timeout := parseDurationManifest(h.Timeout)
	startPeriod := parseDurationManifest(h.StartupGrace)

	var test []string

	switch {
	case len(h.Test) > 0:
		// Raw Docker healthcheck command — use as-is
		test = h.Test
	case h.HTTP != "":
		url := "http://localhost"
		if port > 0 {
			url = "http://localhost:" + itoa(port)
		}
		url += h.HTTP
		test = []string{"CMD-SHELL", "wget -qO- " + url + " || exit 1"}
	default:
		// Default: TCP check on exposed port
		test = []string{"CMD-SHELL", "nc -z localhost " + itoa(port) + " || exit 1"}
	}

	return &runtime.Healthcheck{
		Test:        test,
		Interval:    interval,
		Timeout:     timeout,
		Retries:     h.Retries,
		StartPeriod: startPeriod,
	}
}

func mapRestartPolicy(r string) string {
	switch r {
	case "always":
		return "unless-stopped"
	case "on-failure":
		return "on-failure"
	case "never":
		return "no"
	default:
		return "unless-stopped"
	}
}

func qualifyImageManifest(image string, isPodman bool) string {
	if !isPodman {
		return image
	}
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 {
		first := parts[0]
		if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
			return image
		}
	}
	return "docker.io/" + image
}

func parseDurationManifest(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
