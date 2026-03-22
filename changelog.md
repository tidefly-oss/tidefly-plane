# Changelog

All notable changes to Tidefly Backend will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Authentication — JWT + Argon2id
- Replaced Authboss with custom JWT authentication (golang-jwt/jwt/v5)
- Argon2id password hashing (OWASP 2024 params: 64MB, 3 iterations, 2 threads)
- HttpOnly refresh token cookie (`tfy_rt`) — XSS-safe, never accessible from JS
- Access token in memory only — never stored in localStorage or sessionStorage
- Redis-backed refresh token store with automatic rotation
- Auto-refresh on 401 with singleton promise — prevents parallel refresh calls
- `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `POST /api/v1/auth/refresh`, `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`, `POST /api/v1/auth/change-password`, `POST /api/v1/auth/logout-all`
- `RequireAuthSSE` middleware — reads JWT from `Authorization` header or `?token=` query param (required for EventSource and WebSocket)

#### Caddy Reverse Proxy Integration
- Replaced Traefik with Caddy — all configuration via Admin API, no Caddyfile needed
- `internal/services/caddy` — `CaddyClient` with `Bootstrap()`, `AddHTTPRoute()`, `RemoveRoute()`, `ConfigureTLS()`, `ConfigureInternalTLS()`
- Bootstrap on backend startup: initializes HTTP server config, TLS policies
- Automatic route registration after container deploy (`expose=true`)
- Automatic route removal on container delete
- `tidefly_proxy` Docker network — Caddy connects to deployed containers via dedicated proxy network, isolated from `tidefly_internal` infrastructure
- `ConnectNetwork` / `DisconnectNetwork` added to Runtime interface (Docker + Podman implementations)
- `SetBaseDomain()` — live domain update without restart
- `CADDY_*` environment variables replace all `TRAEFIK_*` variables
- Custom Caddy image (`ghcr.io/tidefly-oss/tidefly-caddy`) with `caddy-l4` and `caddy-ratelimit` plugins

#### Access Control
- Container list filtered by project membership for non-admin users
- Network list filtered by allowed project networks
- Volume list filtered by accessible containers
- `tidefly.internal: "true"` label hides infrastructure containers from all users
- Projects list filtered by user membership (`ListForUser`)

#### Settings — Proxy Domain
- `caddy_base_domain` field in `SystemSettings` — admin can change Control Plane domain live
- `PATCH /api/v1/admin/settings` now accepts `caddy_base_domain`

### Changed

#### Proxy
- `TraefikConfig` replaced with `CaddyConfig` across config, providers, handlers
- `internal/services/traefik/` removed — replaced by `internal/services/caddy/`
- Deploy handlers (Dockerfile, Compose, Template) now register Caddy routes instead of setting Docker labels
- Docker Compose dev: Traefik replaced with tidefly-caddy, non-standard host ports for infra (Postgres: 15432, Redis: 16379, Mailpit: 11025/18025, Caddy: 10080/10443)
- Docker Compose prod: same Caddy setup, Postgres/Redis no host port binding

#### Config
- `SESSION_SECRET` and `COOKIE_SECRET` removed — replaced by `JWT_SECRET`
- `TRAEFIK_*` variables removed — replaced by `CADDY_*` variables
- Config validation updated: checks `JWT_SECRET`, `CADDY_ADMIN_URL`, `CADDY_BASE_DOMAIN`

#### Admin
- `SettingsService` now accepts `*caddy.Client` — domain changes propagate live
- `UpdateSettingsBody` — separate Huma input struct with `omitempty` tags fixes 422 on partial updates
- Registration Mode removed from Settings UI — admin manages users directly

### Fixed
- `helpers.GenerateTempPassword()` migrated from bcrypt to Argon2id — user creation via admin panel now produces login-compatible password hashes
- SSE/WebSocket endpoints (events, logs, stats, exec, notifications) now authenticate via `?token=` query param — fixes 401 for EventSource and WebSocket connections
- Container terminal WebSocket upgrade fixed for Echo v5 — uses `echo.UnwrapResponse()` to get hijackable `http.ResponseWriter`
- Resource limits GET/PATCH now use `api.get`/`api.patch` with Bearer token — fixes 401
- Dashboard SSR disabled — all auth is client-side, fixes 401 on page refresh
- Login cookie `Secure: false` in development (HTTP), `SameSite: Lax` for cross-origin Vite proxy
- Vite proxy configured for same-origin requests — fixes HttpOnly cookie handling in dev
- `auth.init()` called before rendering dashboard children — fixes race condition causing 401 on data requests

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
- Webhook auto-deploy system (GitHub, GitLab, Gitea, Bitbucket, Generic) with HMAC-SHA256
- External notifications via Slack, Discord, SMTP
- Audit logging

---

## Roadmap

### Next Up
- [ ] Multi-node worker support — Plane/Worker model with gRPC tunnel and mTLS
- [ ] Worker Agent binary
- [ ] In-app update button
- [ ] S3-compatible backup integration
- [ ] Custom domain management UI

### Later
- [ ] Two-factor authentication
- [ ] SSO / LDAP integration (Enterprise)

---

[Unreleased]: https://github.com/tidefly-oss/tidefly-backend/compare/v0.0.1-alpha...HEAD
[0.0.1-alpha]: https://github.com/tidefly-oss/tidefly-backend/releases/tag/v0.0.1-alpha