# Changelog

All notable changes to Tidefly Plane will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- Worker selection UI in all deploy wizards

#### Reverse Proxy
- Caddy integration via Admin API — no Caddyfile needed
- Custom Caddy image with `caddy-l4` and `caddy-ratelimit` plugins
- Automatic route registration/removal on container deploy/delete
- Let's Encrypt ACME + internal TLS support
- `tidefly_proxy` Docker network for container routing

#### Monitoring & Observability
- Prometheus metrics registry (`tidefly_*` namespace)
- System metrics: CPU, memory, disk via SSE
- Container count gauges, HTTP instrumentation, job and webhook counters
- Caddy access log streaming via SSE
- Real-time notification stream via SSE

#### Developer Experience
- Huma v2 API with Scalar docs renderer
- OpenAPI tags for all route groups
- golangci-lint v2 configuration
- Wire DI throughout
- Taskfile for all common workflows
- Production Dockerfile (scratch-based, no CGO, multi-arch)
- GitHub Actions CI (lint, test, build)
- GitHub Actions Release (multi-arch Docker push to ghcr.io, GitHub Release)
- In-app update button — checks GitHub Releases API for new versions

#### Other
- Git integration: GitHub, GitLab, Gitea/Forgejo, Bitbucket with AES-256-GCM token encryption
- Webhook auto-deploy (GitHub, GitLab, Gitea, Bitbucket, Generic) with HMAC-SHA256
- External notifications via Slack, Discord, SMTP
- S3 backup integration with Postgres export
- RBAC: admin and member roles with project-scoped permissions
- User management
- Audit logging with retention policies

---

## Roadmap

### Next (Beta)
- [ ] E2E and integration tests
- [ ] Auto-scheduling across worker nodes
- [ ] Custom domain management UI

### Later
- [ ] Two-factor authentication
- [ ] SSO / LDAP integration (Enterprise)

---

[Unreleased]: https://github.com/tidefly-oss/tidefly-plane/compare/v0.0.1-alpha.1...HEAD
[0.0.1-alpha.1]: https://github.com/tidefly-oss/tidefly-plane/releases/tag/v0.0.1-alpha.1


<div align="center">
  <sub>Built with ❤️ by <a href="https://github.com/dbuettgen">@dbuettgen</a> · Part of the <a href="https://github.com/tidefly-oss">tidefly-oss</a> project</sub>
</div>