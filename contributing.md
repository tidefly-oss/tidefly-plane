# Contributing to Tidefly Backend

Thanks for your interest in contributing!

## Getting Started

### Prerequisites

- Go 1.23+
- Docker or Podman
- [Task](https://taskfile.dev) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- [Wire](https://github.com/google/wire) (`go install github.com/google/wire/cmd/wire@latest`)
- [Air](https://github.com/air-verse/air) (`go install github.com/air-verse/air@latest`)

### Setup

```bash
git clone https://github.com/tidefly-oss/tidefly-backend
cd tidefly-backend

task setup        # creates deploy/dev/.env and generates secrets
task dev:up       # starts Postgres, Redis, Traefik, Mailpit
task dev          # starts backend with hot reload
```

## Development Workflow

```bash
task wire         # regenerate Wire DI bindings after changing providers
task test         # run all tests
task lint         # run golangci-lint
task tidy         # go mod tidy
```

## Project Structure

```
cmd/tidefly/          entry point
internal/
  api/              Echo route registration
  bootstrap/        Wire DI providers
  config/           environment config + validation
  handlers/         HTTP handlers
  middleware/        Echo middleware
  models/           GORM models
  services/         business logic
  version/          version info (set via ldflags)
deploy/
  dev/              docker-compose + .env for local dev
  prod/             docker-compose + .env for production
scripts/            helper shell scripts
templates/          service deploy templates
```

## Code Style

- Follow standard Go conventions (`gofmt`, `goimports`)
- Handlers stay thin — business logic belongs in services
- Echo v5 patterns: `c *echo.Context` (pointer), `c.Param()`, `middleware.UserFromContext(c)`
- All new endpoints need an entry in the audit log

## Pull Requests

- Branch from `develop`, not `main`
- Keep PRs focused — one feature or fix per PR
- Add tests for new functionality where possible
- Update `changelog.md` under `[Unreleased]`

## Reporting Security Issues

Please do **not** open a public issue for security vulnerabilities.  
Use [GitHub Private Security Advisories](https://github.com/tidefly-oss/tidefly-backend/security/advisories/new) instead.