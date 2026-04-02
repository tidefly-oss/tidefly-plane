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
    - [Production](#production)
    - [Development](#development)
- [Configuration](#configuration)
- [Tasks](#tasks)
- [Project Structure](#project-structure)
- [Contributing](#contributing)
- [Security](#security)

---

## Deployment

### Production

The recommended way to install Tidefly is via the TUI setup wizard:
```bash
curl -fsSL https://get.tidefly.sh | bash
```

The wizard guides you through server setup, generates secrets, and starts all services automatically.

### Development

#### Prerequisites

- Go 1.26+
- Docker
- [Task](https://taskfile.dev) — `go install github.com/go-task/task/v3/cmd/task@latest`
- [Wire](https://github.com/google/wire) — `go install github.com/google/wire/cmd/wire@latest`
- [Air](https://github.com/air-verse/air) — `go install github.com/air-verse/air@latest`

#### Setup
```bash
git clone https://github.com/tidefly-oss/tidefly-plane
cd tidefly-plane
task setup      # generates deploy/development/.env with secrets
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
task migrate            Run database migrations (dev)
task wire               Regenerate Wire DI bindings
task build              Build production binary
task build:docker       Build Docker image
task lint               Run golangci-lint
task test               Run all tests
task tidy               go mod tidy
task prod:up            Start production stack
task prod:down          Stop production stack
task prod:update        Pull latest images and recreate containers
```

---

## Project Structure
```
cmd/tidefly-plane/        entry point
internal/
  api/
    v1/                   route registration per domain
    v1/proto/agent/       gRPC protobuf + generated code
    adapter/              Echo v5 ↔ Huma adapter
    middleware/           Echo & Huma middleware
    shared/               shared helpers
  bootstrap/              Wire DI providers + wire_gen.go
  ca/                     internal mTLS certificate authority
  config/                 environment config + validation
  jobs/                   asynq background jobs
  metrics/                Prometheus registry
  models/                 GORM models
  services/
    agent/                gRPC server + worker registry
    caddy/                Caddy Admin API client
    git/                  Git integration
    logwatcher/           container log streaming
    notifications/        notification service
    runtime/              Docker/Podman abstraction
    template/             service template loader
    webhook/              webhook delivery
  version/                build version info (set via ldflags)
deploy/
  development/            docker-compose + .env for local dev
  production/             Dockerfile + docker-compose + .env
scripts/
  gen-proto.sh            regenerate protobuf bindings
  init-env.sh             generate .env with random secrets
```

---

## Contributing

See [contributing.md](contributing.md) for setup instructions, code style, and guidelines.

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