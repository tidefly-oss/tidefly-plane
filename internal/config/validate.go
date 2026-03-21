package config

import (
	"errors"
	"fmt"
	"strings"
)

func (c *Config) Validate() error {
	var errs []string

	// ── Hard requirements ─────────────────────────────────
	if c.App.SecretKey == "" {
		errs = append(errs, "APP_SECRET_KEY is required (min 32 chars)")
	} else if len(c.App.SecretKey) < 32 {
		errs = append(errs, fmt.Sprintf("APP_SECRET_KEY too short (%d chars, need 32)", len(c.App.SecretKey)))
	}
	if c.Database.URL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.Auth.JWTSecret == "" {
		errs = append(errs, "JWT_SECRET is required")
	}

	// ── Runtime socket ────────────────────────────────────
	if c.Runtime.SocketPath == "" {
		errs = append(errs, "DOCKER_SOCK (or RUNTIME_SOCKET / PODMAN_SOCKET) is required")
	}

	// ── Caddy cross-validation ────────────────────────────
	if c.Caddy.Enabled {
		if c.Caddy.AdminURL == "" {
			errs = append(errs, "CADDY_ADMIN_URL is required when CADDY_ENABLED=true")
		}
		if c.Caddy.BaseDomain == "" {
			errs = append(errs, "CADDY_BASE_DOMAIN is required when CADDY_ENABLED=true")
		}
		if c.Caddy.ACMEEmail == "" && !c.IsDevelopment() {
			errs = append(errs, "CADDY_ACME_EMAIL is required when CADDY_ENABLED=true in production")
		}
	}

	// ── SMTP prod check ───────────────────────────────────
	if !c.IsDevelopment() {
		if c.SMTP.Host == "localhost" || c.SMTP.Host == "127.0.0.1" {
			errs = append(errs, "SMTP_HOST appears to be a local dev server — set a real SMTP host for production")
		}
	}

	if len(errs) > 0 {
		return errors.New("config validation failed:\n  - " + strings.Join(errs, "\n  - "))
	}

	return nil
}
