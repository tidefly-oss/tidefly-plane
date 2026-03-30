# Tidefly Backend

> Self-hosted container management platform — Docker & Podman, no cloud required.

Tidefly is an open-source alternative to Portainer, Coolify, and Dokploy. This repository contains the Go backend that powers the Tidefly API, background jobs, and deployment engine.

## Stack

- **Go** + Echo v5
- **PostgreSQL** — primary database
- **Redis** — sessions, background jobs (asynq)
- **Traefik** — automatic reverse proxy + SSL
- **Wire** — compile-time dependency injection

## Repositories

| Repo                                                                  | Description                            |
|-----------------------------------------------------------------------|----------------------------------------|
| [tidefly-backend](https://github.com/tidefly-oss/tidefly-backend)     | This repo — Go API + deployment engine |
| [tidefly-ui](https://github.com/tidefly-oss/tidefly-ui)               | SvelteKit frontend                     |
| [tidefly-tui](https://github.com/tidefly-oss/tidefly-tui)             | Bubble Tea setup wizard                |
| [tidefly-templates](https://github.com/tidefly-oss/tidefly-templates) | Service deploy templates               |
| [tidefly-docs](https://github.com/tidefly-oss/tidefly-docs)           | Documentation                          |

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) or [Podman](https://podman.io/getting-started/installation)
- [Task](https://taskfile.dev) — `go install github.com/go-task/task/v3/cmd/task@latest`
- [Go 1.23+](https://go.dev/dl/)
- [Air](https://github.com/air-verse/air) — `go install github.com/air-verse/air@latest`
- [Wire](https://github.com/google/wire) — `go install github.com/google/wire/cmd/wire@latest`

### Setup

```bash
git clone https://github.com/tidefly-oss/tidefly-backend
cd tidefly-plane-backend

task setup       # creates deploy/dev/.env and generates secrets
task dev:up      # starts Postgres, Redis, Traefik, Mailpit
task dev         # starts backend with hot reload (Air)
```

Backend available at `http://localhost:8080`.

### Production

```bash
task setup:prod
task prod:up
```

See [`deploy/prod/`](deploy/prod/) for compose config and environment variables.

## Tasks

```
task setup            Create dev .env and generate secrets
task dev              Start backend with hot reload
task dev:up           Start dev infra (Postgres, Redis, Traefik, Mailpit)
task dev:down         Stop dev infra
task migrate          Run database migrations (dev)
task wire             Regenerate Wire DI bindings
task build            Build production binary
task build:docker     Build Docker image
task test             Run all tests
task lint             Run golangci-lint
task tidy             go mod tidy
```

## Project Structure

```
cmd/tidefly/          entry point
internal/
  api/              route registration
  bootstrap/        Wire DI providers
  config/           environment config + validation
  handlers/         HTTP handlers
  middleware/        Echo middleware
  models/           GORM models
  services/         business logic
  version/          build version info
deploy/
  dev/              local dev compose + .env
  prod/             production compose + .env
scripts/            helper scripts
templates/          service deploy templates
```

## Contributing

See [CONTRIBUTING.md](contributing.md) for setup instructions and guidelines.

## Security

Please do **not** open public issues for security vulnerabilities — use [GitHub Private Security Advisories](https://github.com/tidefly-oss/tidefly-backend/security/advisories/new) instead.

## License

[AGPLv3](LICENSE)