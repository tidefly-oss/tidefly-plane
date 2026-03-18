# Changelog

All notable changes to Tidefly Backend will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Notifications — Slack, Discord & Email
- External push notifications via Slack Incoming Webhooks, Discord Webhooks, and SMTP email
- `internal/services/notifier` — new `Service` with `Send()`, `Test()`, `sendSlack()`, `sendDiscord()`, `sendEmail()`
- Configurable triggers per event: deploy success, container down (ERROR/FATAL from LogWatcher), webhook delivery failure
- `SystemSettings` model extended with: `notifications_enabled`, `slack_webhook_url`, `discord_webhook_url`, `notify_on_deploy`, `notify_on_container_down`, `notify_on_webhook_fail`
- `notifications.Service.Publish()` — generic in-app notification for system-level events
- `notifications.Service.IsNew()` — dedup guard: only fires external push on first occurrence, not on backend restart replay
- `notifications.Fingerprint()` — exported for use in LogWatcher dedup check
- Logger extended with `Errorw()` and `Warnw()` — structured key-value logging without requiring an `error` value
- `POST /api/v1/admin/settings/test/{channel}` — test endpoint for notification channels

#### Webhooks — Auto-deploy on Git push
- Full webhook management: create, list, get, update, delete per project
- HMAC-SHA256 request signing with per-webhook encrypted secrets (AES-256-GCM)
- Secret rotation via `POST /api/v1/projects/{pid}/webhooks/{id}/rotate`
- Delivery history with status, commit, branch, duration, and error details
- Async webhook dispatch via asynq background jobs (`webhooks` queue)
- Provider support: GitHub, GitLab, Gitea/Forgejo, Bitbucket, Generic
- Branch filter — trigger only on specific branches or `*` for all
- Trigger types: `redeploy` (existing service) and `deploy` (fresh from template)

#### Version Tracking
- `internal/version/version.go` — `var Version = "0.1.0-alpha"` overridable via ldflags at build time
- `GET /api/v1/system/info` now returns `tidefly_version` field
- Build task sets version from `git describe --tags --always --dirty`

#### Traefik Integration
- Automatic reverse proxy + SSL for deployed services via Let's Encrypt HTTP-Challenge
- Zero-config HTTPS: user sets a wildcard DNS record once, every deployed service gets a subdomain automatically
- `TRAEFIK_*` environment variables control the full integration
- `expose=true` + `port` fields on Dockerfile deploy, Compose deploy, and template deploy requests
- `done` event on deploy includes `url` field with the public HTTPS URL
- ACME staging CA support for testing without rate limits
- HTTP to HTTPS redirect middleware auto-generated when `TRAEFIK_FORCE_HTTPS=true`
- Custom domain support via `custom_domain` on deploy

#### Audit Logging
- `AuditC(c, action, resourceID, err, details)` — single-call helper that extracts user_id, ip, user_agent from Echo context
- New audit actions across containers, deploy, git, networks, projects, volumes, admin, auth
- Both success and failure paths logged with structured details

#### Request/Response Logging Middleware
- `middleware.RequestLogger` — production-grade HTTP logging via slog
- Base fields on every request: method, path, query, status, latency_ms, ip, request_id, response_bytes
- Extended fields only on 4xx/5xx or slow requests
- Sensitive field redaction from JSON bodies: password, token, secret, api_key, authorization, csrf_token
- `wrappedWriter` implements `http.Flusher` so SSE streams work through the middleware

#### Config Validation
- `config.Validate()` called at startup — hard fail with clear error list if required fields missing
- Required: `APP_SECRET_KEY` (min 32 chars), `DATABASE_URL`, `SESSION_SECRET`, `COOKIE_SECRET`, `DOCKER_SOCK`
- Traefik cross-validation: `TRAEFIK_BASE_DOMAIN` required when enabled
- SMTP cross-validation: warns in production when `SMTP_HOST` is still `localhost`

#### SMTP — User-provided Mail Server
- `SMTPConfig` expanded with `User`, `Password`, `From`, `TLS` fields
- In dev: Mailpit defaults (`localhost:1025`, no auth)
- In prod: user enters their own SMTP credentials (Resend, Postmark, own server)

#### Dev/Prod Compose Split
- `deploy/dev/docker-compose.yaml` — infra only: Traefik, Postgres, Redis, Mailpit
- `deploy/prod/docker-compose.yaml` — full stack: Traefik, Postgres, Redis, Backend, Frontend
- No port binding for Postgres/Redis in prod (internal network only)

### Changed

#### HTTP Server
- Replaced `echo.StartConfig` with raw `net/http.Server` — timeouts set to `0` so SSE and WebSocket streams are not terminated

#### SSE Streams — Context Fix
- `events.Stream` and `notifications.Stream` now derive their own `context.WithCancel(context.Background())` instead of using the request context directly
- Client disconnect detected via goroutine watching `c.Request().Context().Done()`

#### S3 Storage
- Removed MinIO-specific integration — replaced with generic S3-compatible endpoint support
- Works with AWS S3, Cloudflare R2, Backblaze B2, or any S3-compatible provider
- Configured via `S3_ENDPOINT`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`

### Fixed
- SSE endpoints no longer disconnect after 30 seconds
- `POST /auth/login` now correctly sets session cookie
- `GET /api/v1/auth/me` no longer returns 401 after successful login
- Images and networks "used by containers" fetch no longer 404
- Request logger body truncation at 2KB caused HMAC signature mismatch for webhook payloads — increased to 64KB
- Deploy handler nil pointer dereference on `*bool Expose` field when not provided — added nil check
- LogWatcher external notifications no longer re-fire on backend restart for already-seen errors — `IsNew()` guard added

---

## [0.0.1-alpha] - TBD

> First internal alpha. Core container management, deployment workflows, monitoring, and authentication.

### Added
- Initial project structure (Go/Echo v5, PostgreSQL, Redis, asynq)
- Docker and Podman runtime support with auto-detection
- Container management — list, start, stop, restart, remove
- Dockerfile and Docker Compose deployment wizards
- Project-based container isolation with dedicated Docker networks
- Real-time container metrics (CPU, memory, disk) with historical charts
- System monitoring with alert thresholds and SSE notifications
- Interactive terminal via WebSocket
- Container resource limits with live update support
- Port conflict detection
- Background cleanup jobs (asynq/Redis)
- Audit logging with GDPR-compliant retention policies
- Service template system (YAML-based)
- Git integration: GitHub, GitLab, Gitea/Forgejo, Bitbucket with AES-256-GCM token encryption
- RBAC: admin and member roles with project-scoped permissions
- User management
- Authentication (session-based, bcrypt, Redis session store)

---

## Roadmap

### Next Up
- [ ] In-app update button — check GitHub releases, notify and update on the fly
- [ ] Multi-node worker support — hub-and-worker model over WireGuard VPN
- [ ] S3-compatible backup integration
- [ ] Custom domain management UI

### Later
- [ ] Two-factor authentication
- [ ] SSO / LDAP integration (Enterprise)

---

[Unreleased]: https://github.com/tidefly-oss/tidefly-backend/compare/v0.0.1-alpha...HEAD
[0.0.1-alpha]: https://github.com/tidefly-oss/tidefly-backend/releases/tag/v0.0.1-alpha