package config

import (
	"errors"
	"fmt"
	"strings"
)

func (c *Config) Validate() error {
	var errs []string

	// ── Hard requirements ────────────────────────────────
	if c.App.SecretKey == "" {
		errs = append(errs, "APP_SECRET_KEY is required (min 32 chars)")
	} else if len(c.App.SecretKey) < 32 {
		errs = append(errs, fmt.Sprintf("APP_SECRET_KEY too short (%d chars, need 32)", len(c.App.SecretKey)))
	}
	if c.Database.URL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.Auth.SessionSecret == "" {
		errs = append(errs, "SESSION_SECRET is required")
	}
	if c.Auth.CookieSecret == "" {
		errs = append(errs, "COOKIE_SECRET is required")
	}

	// ── API Docs & Traefik Dashboard ─────────────────────
	if !c.IsDevelopment() {
		if !c.App.DocsEnabled {
			errs = append(errs, "API_DOCS_ENABLED=false is not allowed in production")
		}
		if !c.Traefik.DashboardEnabled && c.Traefik.Enabled {
			errs = append(
				errs,
				"TRAEFIK_DASHBOARD_ENABLED=false is not allowed in production when TRAEFIK_ENABLED=true",
			)
		}
	}

	// ── Runtime socket ───────────────────────────────────
	if c.Runtime.SocketPath == "" {
		errs = append(errs, "DOCKER_SOCK (or RUNTIME_SOCKET / PODMAN_SOCKET) is required")
	}

	// ── Traefik cross-validation ─────────────────────────
	if c.Traefik.Enabled {
		if c.Traefik.BaseDomain == "" {
			errs = append(errs, "TRAEFIK_BASE_DOMAIN is required when TRAEFIK_ENABLED=true")
		}
		if c.Traefik.ACMEEmail == "" && !c.IsDevelopment() {
			errs = append(errs, "TRAEFIK_ACME_EMAIL is required when TRAEFIK_ENABLED=true in production")
		}
	}

	// ── SMTP prod check ──────────────────────────────────
	if !c.IsDevelopment() {
		if c.SMTP.Host == "localhost" || c.SMTP.Host == "127.0.0.1" {
			errs = append(
				errs,
				"SMTP_HOST appears to be a local dev server (Mailpit?) — set a real SMTP host for production",
			)
		}
	}

	if len(errs) > 0 {
		return errors.New("config validation failed:\n  - " + strings.Join(errs, "\n  - "))
	}

	return nil
}
