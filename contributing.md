# Contributing to Tidefly Plane

Thanks for your interest in contributing!

## Getting Started

### Prerequisites

- Go 1.26+
- Docker (and optionally Podman — always test with both runtimes)
- [Task](https://taskfile.dev) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- [Wire](https://github.com/google/wire) (`go install github.com/google/wire/cmd/wire@latest`)
- [Air](https://github.com/air-verse/air) (`go install github.com/air-verse/air@latest`)
- [protoc](https://grpc.io/docs/protoc-installation/) + Go plugins (only needed when modifying the agent gRPC protocol)
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Setup
```bash
git clone https://github.com/tidefly-oss/tidefly-plane
cd tidefly-plane
task setup        # creates deploy/development/.env and generates secrets
task dev:up       # starts Postgres, Redis, Caddy
task dev          # starts backend with hot reload (runs wire first)
```

## Development Workflow
```bash
task wire         # regenerate Wire DI bindings after changing providers
task test         # run all tests
task lint         # run golangci-lint
task tidy         # go mod tidy
```

### Modifying the Agent Protocol

If you change `internal/api/v1/proto/agent/agent.proto`, regenerate the Go bindings:
```bash
bash scripts/gen-proto.sh
```

Commit the generated `*.pb.go` and `*_grpc.pb.go` files alongside your proto changes.

## Project Structure
```
cmd/tidefly-plane/        entry point
internal/
  api/
    v1/                   route registration per domain
    v1/proto/agent/       gRPC protobuf definitions + generated code
    adapter/              Echo v5 ↔ Huma adapter
    middleware/           Echo & Huma middleware
    shared/               shared helpers (Op() factory)
  bootstrap/              Wire DI providers + wire_gen.go
  ca/                     internal mTLS certificate authority
  config/                 environment config + validation
  jobs/                   asynq background jobs
  logger/                 structured slog wrapper
  metrics/                Prometheus registry
  models/                 GORM models
  services/
    agent/                gRPC server + worker registry + client
    caddy/                Caddy Admin API client
    git/                  Git integration service
    logwatcher/           container log streaming
    notifications/        notification service
    notifier/             external notification delivery (Slack, Discord, SMTP)
    runtime/              Docker/Podman runtime abstraction
    template/             service template loader
    webhook/              webhook delivery service
  version/                version info (set via ldflags at build time)
deploy/
  development/            docker-compose + .env for local dev
  production/             Dockerfile + docker-compose + .env for production
scripts/
  gen-proto.sh            regenerate protobuf bindings
  init-env.sh             generate .env with random secrets
```

## Code Style

- Follow standard Go conventions (`gofmt`, `goimports`)
- Handlers stay thin — business logic belongs in services
- Echo v5 patterns: `c *echo.Context` (pointer), `c.Param()`, `middleware.UserFromContext(c)`
- Huma routes use `shared.Op(id, method, path, summary, tag, mw...)` — never inline `huma.Operation` unless you need `DefaultStatus`
- All new endpoints need an entry in the audit log
- No CGO — all code must compile with `CGO_ENABLED=0`

## Pull Requests

- Branch from `develop`, not `main`
- Keep PRs focused — one feature or fix per PR
- Add tests for new functionality where possible
- Update `changelog.md` under `[Unreleased]`

## Reporting Security Issues

Please do **not** open a public issue for security vulnerabilities.  
Use [GitHub Private Security Advisories](https://github.com/tidefly-oss/tidefly-plane/security/advisories/new) instead.