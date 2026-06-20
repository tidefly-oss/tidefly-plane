# Changelog

All notable changes to Tidefly Plane will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.1-beta.21] - 2026-06-20

> Internal architecture refactor + security hardening. No functional changes to the API surface.

### Changed

#### Package Structure
- Feature-first flat layout — all features live directly under `internal/`, replacing the layered `api/v1/handlers/*/http` + `domain/*/` structure
- `internal/bootstrap/` replaces `internal/platform/bootstrap/`
- `internal/logmon/` replaces `internal/infra/logwatcher/`
- `internal/converter/` promoted from `internal/manifest/converter/` to break import cycles
- `internal/queue/` introduced — zero-internal-imports package for all asynq enqueue helpers, breaks the `manifest ↔ jobs ↔ manifest/converter` cycle
- `internal/access/` introduced — centralized access control helpers shared by `container`, `network`, `volume` — eliminates duplication and `network → container` import
- `internal/platform/config/` consolidated from 15 files to 4
- `internal/infra/agent/` merged into `internal/agent/` — gRPC server/client/registry alongside HTTP handler

#### Naming
- All `New()` constructors renamed to `NewHandler()`
- All Huma handler methods made unexported — `h.List` → `h.list`
- All Input/Output types made unexported — `GitListOutput` → `listOutput`
- `auth.Service` → `auth.JWTService`

#### Routes
- All route paths use `httpx.V1` constant instead of hardcoded `"/api/v1"`

#### Import cycle fixes
- `middleware` has no dependency on `auth` — bootstrap wires via `jwtValidator()` adapter
- `access` has no dependency on `middleware` — bootstrap wires via `access.SetUserReader()`
- `queue` imports nothing from `internal/` — payload types duplicated as standalone structs
- `manifest` no longer imports `jobs` — uses `queue.Enqueue*` directly

### Security

- **HTTP server timeouts** — `ReadTimeout: 15s`, `WriteTimeout: 60s`, `IdleTimeout: 120s`, `MaxHeaderBytes: 1MB` — prevents Slowloris and header-bombing attacks
- **Global rate limiting** — `RateLimitAPI()` (300 req/min per IP) applied to all routes
- **Auth rate limiting** — `RateLimitAuth()` (10 req/min per IP) applied to all `/api/v1/auth/*` routes via chi group
- **Token length check** — JWT tokens over 2048 bytes rejected before parsing
- **IP extraction hardened** — `realIP()` correctly strips port from `RemoteAddr`, handles `X-Forwarded-For` multi-value
- **Tidefly label constants** centralized in `access/` — consistent filtering of internal containers/networks/volumes across all packages

---

## [0.0.1-beta.1] - 2026-05-20

> First beta release. Orchestration API surface, cascading resource cleanup, self-healing improvements, and admin settings enhancements.

### Added

#### Orchestration API
- Unified `ServiceView` response on `GET /services` and `GET /services/:id` combining desired state (manifest/DB) with live runtime state (Docker/Podman)
- `BuildView()` in `ServiceManager` merges live container state (status, replicas) with manifest desired state
- `DriftState` exposes replica drift and not-running state per service

#### Resource Cleanup
- Cascading async cleanup on service delete via `TaskServiceCleanup` job
- Collects associated images and volumes before container removal
- Safely skips shared resources still referenced by other containers

#### Self-Healing
- Orphan purge in `HandleServiceHealthCheck`: services with no manifest and no container are automatically deleted
- Stuck-deploying purge: services stuck in `deploying` for > 10 minutes with no container are removed

#### Admin Settings
- `api_docs_enabled` toggle in admin settings
- `GuardDocs` middleware: blocks `/docs` and `/openapi` live from DB — no restart required

### Fixed
- Suppress self-heal log noise for already-deleted services
- Extract `proxyNetwork` constant in jobs package

### Changed
- `ServiceJobHandler` carries `asynq.Client` for cleanup job enqueueing

---

## [0.0.1-alpha.1] - 2026-03-31

> First public alpha. Core container management, deployment workflows, monitoring, authentication, and multi-node worker support via gRPC mTLS tunnel.

### Added

#### Authentication
- JWT + Argon2id authentication (OWASP 2024 params)
- HttpOnly refresh token cookie (`tfy_rt`) — XSS-safe
- Redis-backed refresh token store with automatic rotation
- `POST /api/v1/auth/register`, `login`, `refresh`, `logout`, `logout-all`
- `GET /api/v1/auth/me`, `POST /api/v1/auth/change-password`
- `RequireAuthSSE` middleware for EventSource and WebSocket endpoints

#### Container Management
- Docker and Podman runtime support with auto-detection
- Container list, start, stop, restart, delete
- Dockerfile and Docker Compose deployment wizards
- Service template system (YAML-based)
- Project-based container isolation via dedicated Docker networks
- Container resource limits with live update support
- Interactive terminal via WebSocket
- Real-time container logs via SSE
- Real-time container metrics (CPU, memory) via SSE
- Port conflict detection
- Background cleanup jobs (asynq/Redis)

#### Multi-Node Worker Support
- Plane/Agent architecture with gRPC bidirectional stream over mTLS
- Custom internal CA — issues and renews mTLS certificates for worker nodes
- Worker registration via token — `POST /api/v1/agent/register`
- Certificate renewal — `POST /api/v1/agent/renew`
- Worker node management — list, revoke, delete
- Container management on worker nodes — list containers, stream logs
- Caddy routing via worker IP for worker-deployed containers

#### Reverse Proxy
- Caddy integration via Admin API — no Caddyfile needed
- Automatic route registration/removal on container deploy/delete
- Let's Encrypt ACME + internal TLS support

#### Monitoring & Observability
- Prometheus metrics registry (`tidefly_*` namespace)
- System metrics: CPU, memory, disk via SSE
- Caddy access log streaming via SSE
- Real-time notification stream via SSE

#### Developer Experience
- Huma v2 API with Scalar docs renderer
- Wire DI throughout
- Taskfile for all common workflows
- Production Dockerfile (scratch-based, no CGO, multi-arch)
- GitHub Actions CI (lint, test, build, Docker push)

#### Other
- Git integration: GitHub, GitLab, Gitea/Forgejo, Bitbucket with AES-256-GCM token encryption
- Webhook auto-deploy with HMAC-SHA256 signature verification
- External notifications via Slack, Discord, SMTP
- S3 backup integration with Postgres export
- RBAC: admin and member roles with project-scoped permissions
- Audit logging with retention policies

---

## Roadmap

### Next
- [ ] E2E and integration tests
- [ ] Auto-scheduling across worker nodes
- [ ] Custom domain management UI
- [ ] Two-factor authentication

### Done
- [x] Feature-first flat package layout
- [x] Import cycle resolution via `queue/` + `access/` packages
- [x] HTTP server timeouts + rate limiting
- [x] Unified orchestration API
- [x] Cascading resource cleanup on service delete
- [x] Self-healing orphan and stuck-deploy purge
- [x] API docs toggle in admin settings

---

[0.0.1-beta.2]: https://github.com/tidefly-oss/tidefly-plane/compare/v0.0.1-beta.1...v0.0.1-beta.2
[0.0.1-beta.1]: https://github.com/tidefly-oss/tidefly-plane/compare/v0.0.1-alpha.1...v0.0.1-beta.1
[0.0.1-alpha.1]: https://github.com/tidefly-oss/tidefly-plane/releases/tag/v0.0.1-alpha.1

<div align="center">
  <sub>Built with ❤️ by <a href="https://github.com/dbuettgen">@dbuettgen</a> · Part of the <a href="https://github.com/tidefly-oss">tidefly-oss</a> project</sub>
</div>