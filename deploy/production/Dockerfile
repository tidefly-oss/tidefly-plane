# =============================================================================
# Build Stage
# =============================================================================
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

# Dependencies mit Go module cache (bleibt zwischen Builds gecacht)
COPY go.mod go.sum ./
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    go mod download && go mod verify

# Wire installieren (gecacht via GOPATH cache)
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    go install github.com/google/wire/cmd/wire@latest

# Source kopieren
COPY . .

# Wire Bindings generieren + Binary bauen
ARG VERSION=v0.0.1-alpha.1
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    wire ./internal/bootstrap/ && \
    mkdir -p /out && \
    CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags="-s -w \
        -X github.com/tidefly-oss/tidefly-plane/internal/version.Version=${VERSION} \
        -X github.com/tidefly-oss/tidefly-plane/internal/version.Commit=${COMMIT} \
        -X github.com/tidefly-oss/tidefly-plane/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/tidefly-plane \
      ./cmd/tidefly-plane

# =============================================================================
# Runtime Stage — scratch
# =============================================================================
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

WORKDIR /app

COPY --from=builder /out/tidefly-plane .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="tidefly-plane" \
      org.opencontainers.image.description="Tidefly Plane — container management backend" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/tidefly-oss/tidefly-plane" \
      org.opencontainers.image.licenses="AGPL-3.0"

EXPOSE 8181 7443

ENTRYPOINT ["/app/tidefly-plane"]