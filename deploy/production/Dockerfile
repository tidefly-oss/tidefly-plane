# =========================
# Build Stage
# =========================
FROM golang:1.26.1-alpine AS builder

# Arbeitsverzeichnis
WORKDIR /app

# Abhängigkeiten zuerst (Cache Layer)
COPY go.mod go.sum ./
RUN apk add git build-base ca-certificates \
    && go mod download

# Quellcode kopieren
COPY . .

# Statisches Binary bauen (ohne CGO für kleinere Größe)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tidefly ./cmd/tidefly

# =========================
# Runtime Stage
# =========================
FROM alpine:3.18

WORKDIR /app

# Binary vom Builder kopieren
COPY --from=builder /app/tidefly .

# HTTPS Zertifikate
RUN apk add --no-cache ca-certificates

# Expose Port
EXPOSE 8181

# Start des Binaries
CMD ["./tidefly"]