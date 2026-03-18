package traefik

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tidefly-oss/tidefly-backend/internal/config"
)

// ServiceConfig describes how a container should be exposed via Traefik.
type ServiceConfig struct {
	// Name is used as router/service identifier and default subdomain.
	// Sanitized automatically (lowercase, alphanumeric + dash only).
	Name string
	// Port is the container-internal port Traefik forwards to. Required.
	Port int
	// CustomDomain overrides the auto-generated {name}.{BaseDomain} subdomain.
	// The user must point this domain to the server themselves (A or CNAME).
	CustomDomain string
}

var nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeName produces a valid Traefik router name from an arbitrary string.
func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = nonAlphanumDash.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// LabelsFor returns the Docker labels needed to expose a container via Traefik.
//
// Returns an empty map (safe to merge) when:
//   - Traefik integration is disabled
//   - BaseDomain is empty
//   - Port is 0
//   - Name sanitizes to empty string
//
// Generated label sets:
//   - HTTP router with optional HTTP→HTTPS redirect (when ForceHTTPS=true)
//   - HTTPS router with TLS + Let's Encrypt certresolver (when ForceHTTPS=true)
//   - Service with loadbalancer pointing to the container port
func LabelsFor(cfg config.TraefikConfig, svc ServiceConfig) map[string]string {
	if !cfg.Enabled || cfg.BaseDomain == "" || svc.Port == 0 {
		return map[string]string{}
	}

	name := sanitizeName(svc.Name)
	if name == "" {
		return map[string]string{}
	}

	host := fmt.Sprintf("%s.%s", name, cfg.BaseDomain)
	if svc.CustomDomain != "" {
		host = svc.CustomDomain
	}

	labels := map[string]string{
		"traefik.enable": "true",

		// Service — points to the container port
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", name): fmt.Sprintf("%d", svc.Port),
	}

	if cfg.ForceHTTPS {
		// HTTP router — only redirects to HTTPS
		labels[fmt.Sprintf("traefik.http.routers.%s-http.rule", name)] =
			fmt.Sprintf("Host(`%s`)", host)
		labels[fmt.Sprintf("traefik.http.routers.%s-http.entrypoints", name)] =
			cfg.EntrypointHTTP
		labels[fmt.Sprintf("traefik.http.routers.%s-http.middlewares", name)] =
			fmt.Sprintf("%s-redirect@docker", name)
		labels[fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectscheme.scheme", name)] =
			"https"
		labels[fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectscheme.permanent", name)] =
			"true"

		// HTTPS router — handles traffic + TLS
		labels[fmt.Sprintf("traefik.http.routers.%s.rule", name)] =
			fmt.Sprintf("Host(`%s`)", host)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", name)] =
			cfg.EntrypointHTTPS
		labels[fmt.Sprintf("traefik.http.routers.%s.tls", name)] =
			"true"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", name)] =
			"letsencrypt"
		labels[fmt.Sprintf("traefik.http.routers.%s.service", name)] =
			name
	} else {
		// HTTP only — single router
		labels[fmt.Sprintf("traefik.http.routers.%s-http.rule", name)] =
			fmt.Sprintf("Host(`%s`)", host)
		labels[fmt.Sprintf("traefik.http.routers.%s-http.entrypoints", name)] =
			cfg.EntrypointHTTP
		labels[fmt.Sprintf("traefik.http.routers.%s-http.service", name)] =
			name
	}

	return labels
}

// Domain returns the auto-generated public domain for a service name.
// Returns empty string when Traefik is disabled or BaseDomain is unset.
func Domain(cfg config.TraefikConfig, name string) string {
	if !cfg.Enabled || cfg.BaseDomain == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", sanitizeName(name), cfg.BaseDomain)
}

// MergeLabels merges Traefik labels into an existing label map.
// Safe to call even when LabelsFor returns an empty map.
func MergeLabels(existing map[string]string, traefikLabels map[string]string) map[string]string {
	if existing == nil {
		existing = make(map[string]string)
	}
	for k, v := range traefikLabels {
		existing[k] = v
	}
	return existing
}
