<p align="center">
  <img src="https://raw.githubusercontent.com/tidefly-oss/.github/main/assets/tidefly_mascot.svg" width="320" alt="Tidefly" />
</p>

<p align="center">
  <strong>Go backend powering the Tidefly API, deployment engine, and worker management.</strong>
</p>

<p align="center">
  <a href="https://github.com/tidefly-oss/tidefly-plane/releases"><img src="https://img.shields.io/github/v/release/tidefly-oss/tidefly-plane?include_prereleases&label=version&color=7c3aed" alt="Version" /></a>
  <a href="https://github.com/tidefly-oss/tidefly-plane/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-AGPLv3-06b6d4" alt="License" /></a>
  <a href="https://github.com/tidefly-oss/tidefly-plane/actions"><img src="https://img.shields.io/github/actions/workflow/status/tidefly-oss/tidefly-plane/ci.yaml?branch=main&label=CI" alt="CI" /></a>
</p>

---

## Table of Contents

- [Deployment](#deployment)
- [Configuration](#configuration)
- [Tasks](#tasks)
- [Project Structure](#project-structure)
- [Code Style](#code-style)
- [Contributing](#contributing)
- [Security](#security)

---

## Deployment

### Production

The recommended way to install Tidefly is via the TUI setup wizard:
```bash
curl -fsSL https://raw.githubusercontent.com/tidefly-oss/tidefly-tui/main/scripts/install.sh | bash
```

### Development

#### Prerequisites

- Go 1.24+
- Docker
- [Task](https://taskfile.dev) — `go install github.com/go-task/task/v3/cmd/task@latest`
- [Wire](https://github.com/google/wire) — `go install github.com/google/wire/cmd/wire@latest`
- [Air](https://github.com/air-verse/air) — `go install github.com/air-verse/air@latest`

#### Setup
```bash
git clone https://github.com/tidefly-oss/tidefly-plane
cd tidefly-plane
task setup      # generates manifest/development/.env with secrets
task dev:up     # starts Postgres, Redis, Caddy
task dev        # starts backend with hot reload
```

Backend available at `http://localhost:8181`.

---

## Configuration

All configuration is done via environment variables. Run `task setup` to generate a `.env` with secure defaults.

See [`.env.example`](.env.example) for all available options.

---

## Tasks
```
task setup              Generate dev .env and secrets
task dev                Start backend with hot reload
task dev:up             Start dev infra (Postgres, Redis, Caddy)
task dev:down           Stop dev infra
task dev:reset          Stop dev infra and delete all volumes
task wire               Regenerate Wire DI bindings
task build              Build production binary
task build:docker       Build Docker image
task lint               Run golangci-lint
task test               Run all tests
task tidy               go mod tidy
```

---

## Project Structure

The codebase follows a **feature-first flat layout** based on the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md). Every feature lives in its own package directly under `internal/` — no layered `handlers/services/repositories` nesting.

```
cmd/tidefly-plane/      entry point (main.go)

internal/
  ├── platform/         cross-cutting concerns (no business logic)
  │   ├── ca/           internal mTLS certificate authority
  │   ├── config/       environment config + validation
  │   ├── crypto/       AES-256-GCM encryption helpers
  │   ├── eventbus/     in-process pub/sub (WebSocket fanout)
  │   ├── logger/       structured logging + audit + DB log
  │   ├── metrics/      Prometheus registry
  │   ├── secret/       secret management
  │   └── version/      build version (set via ldflags)
  │
  ├── infra/            adapters for external systems
  │   ├── caddy/        Caddy Admin API client
  │   ├── database/     GORM connect + AutoMigrate
  │   ├── ingress/      ingress adapter interface + Caddy impl
  │   ├── redis/        Redis connect
  │   └── runtime/      Docker/Podman abstraction
  │
  ├── access/           shared access control — label constants, network/container filtering
  ├── bootstrap/        Wire DI providers + wire_gen.go + App wiring
  ├── middleware/       HTTP + Huma middleware (auth, rate limiting, logging, CORS…)
  ├── models/           GORM models (shared across features)
  ├── queue/            asynq enqueue helpers (zero internal imports — breaks cycles)
  ├── logmon/           container log monitor (background service)
  │
  └── [feature packages — one per domain, flat under internal/]
      ├── admin/        user + settings management
      ├── agent/        gRPC server, client, registry + HTTP handler
      ├── auth/         JWT, sessions, password hashing
      ├── backup/       S3 backup
      ├── container/    container lifecycle + SSE streams + exec
      ├── converter/    manifest/compose/dockerfile → ServiceManifest
      ├── dashboard/    aggregated overview endpoint
      ├── deploy/       deploy orchestration (Deployer)
      ├── events/       SSE event stream
      ├── git/          Git integrations (GitHub, GitLab, Gitea, Bitbucket)
      ├── image/        container image management
      ├── log/          app + audit log endpoints
      ├── manifest/     ServiceManifest handler + manager
      ├── network/      Docker network management
      ├── notification/ in-app + external notifications
      ├── project/      project management
      ├── setup/        initial admin setup
      ├── system/       health, metrics, version check, self-update
      ├── template/     service template loader (live from GitHub)
      ├── volume/       Docker volume management
      ├── webhook/      webhook CRUD + inbound receiver
      └── ws/           WebSocket handler

deploy/
  development/          docker-compose + .env for local dev
  production/           Dockerfile + docker-compose + .env
```

---

## Code Style

We follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) with these project-specific rules:

**Package naming**
- Lowercase, singular, no stuttering (`container` not `containers`, `git` not `gitservice`)
- Short and descriptive (`infra` not `infrastructure`, `logmon` not `logwatcher`)
- No `http`, `service`, `handler`, `repository` sub-packages — everything lives flat in the feature package

**Package structure (per feature)**
- `handler.go` — `Handler` struct + `NewHandler()`
- `store.go` — DB queries (replaces `repository.go`)
- `routes.go` — `RegisterRoutes()` with `httpx.V1` constant
- `errors.go` — huma error helpers if needed
- Additional files named after what they do (`integrations.go`, `stream.go`)

**Constructors**
- Always `NewHandler()`, never `New()` — avoids `git.New()` ambiguity

**Handler methods**
- Unexported (lowercase) — `h.list`, `h.create`, `h.delete`
- Input/Output types also unexported — `listInput`, `createOutput`

**Routes**
- Always use `httpx.V1` constant: `httpx.V1+"/containers"` not `"/api/v1/containers"`

**Import cycles**
- `queue/` has zero internal imports — all enqueue helpers live here
- `access/` has zero dependency on `middleware/` — bootstrap wires via `access.SetUserReader()`
- `middleware/` has zero dependency on `auth/` — bootstrap wires via `jwtValidator()` adapter
- `platform/` packages never import feature packages

**Label constants**
- Always use `access.LabelManaged`, `access.LabelInternal` etc. — never hardcode `"tidefly.internal"` strings

---

## Contributing

See [contributing.md](contributing.md) for setup instructions and guidelines.

---

## Security

Please do **not** open public issues for security vulnerabilities.
Use [GitHub Private Security Advisories](https://github.com/tidefly-oss/tidefly-plane/security/advisories/new) instead.

## License

AGPLv3 — see [LICENSE](LICENSE)

---

<div align="center">
  <sub>Built with ❤️ by <a href="https://github.com/dbuettgen">@dbuettgen</a> · Part of the <a href="https://github.com/tidefly-oss">tidefly-oss</a> project</sub>
</div>