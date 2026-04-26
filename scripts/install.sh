#!/usr/bin/env bash
# =============================================================================
# Tidefly Plane — Install Script
# Aufruf:   curl -fsSL https://get.tidefly.sh | bash
# oder:     bash install.sh [--version 0.1.0] [--dir /opt/tidefly-plane] [--no-start]
#
# Exit Codes:
#   0 — Erfolg
#   1 — Allgemeiner Fehler
#   2 — Fehlende Abhängigkeit
#   3 — .env bereits vorhanden (skip, kein Fehler wenn --skip-existing)
# =============================================================================
set -euo pipefail

# =============================================================================
# Konfiguration
# =============================================================================
TIDEFLY_VERSION="${TIDEFLY_VERSION:-latest}"
TIDEFLY_DIR="${TIDEFLY_DIR:-/opt/tidefly-plane}"
TIDEFLY_REPO="tidefly"
COMPOSE_FILE="$TIDEFLY_DIR/docker-compose.yaml"
ENV_FILE="$TIDEFLY_DIR/.env"
REDIS_DIR="$TIDEFLY_DIR/redis"

# Feature Flags (von TUI oder CLI überschreibbar)
OPT_NO_START="${OPT_NO_START:-false}"
OPT_SKIP_EXISTING="${OPT_SKIP_EXISTING:-false}"
OPT_WITH_UI="${OPT_WITH_UI:-false}"
OPT_NON_INTERACTIVE="${OPT_NON_INTERACTIVE:-false}"

# =============================================================================
# Farben / Output
# =============================================================================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
RESET='\033[0m'

log_info()    { echo -e "${BLUE}[tidefly]${RESET} $*"; }
log_success() { echo -e "${GREEN}[tidefly]${RESET} $*"; }
log_warn()    { echo -e "${YELLOW}[tidefly]${RESET} $*"; }
log_error()   { echo -e "${RED}[tidefly]${RESET} $*" >&2; }
log_step()    { echo -e "\n${BOLD}── $* ${RESET}"; }

# =============================================================================
# CLI Args parsen
# =============================================================================
parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)         TIDEFLY_VERSION="$2"; shift 2 ;;
      --dir)             TIDEFLY_DIR="$2"; shift 2 ;;
      --no-start)        OPT_NO_START=true; shift ;;
      --with-ui)         OPT_WITH_UI=true; shift ;;
      --non-interactive) OPT_NON_INTERACTIVE=true; shift ;;
      --skip-existing)   OPT_SKIP_EXISTING=true; shift ;;
      --help|-h)         usage; exit 0 ;;
      *) log_error "Unbekanntes Argument: $1"; usage; exit 1 ;;
    esac
  done
  COMPOSE_FILE="$TIDEFLY_DIR/docker-compose.yaml"
  ENV_FILE="$TIDEFLY_DIR/.env"
  REDIS_DIR="$TIDEFLY_DIR/redis"
}

usage() {
  cat <<EOF
Usage: install.sh [OPTIONS]

  --version <tag>       Image-Version (default: latest)
  --dir <path>          Installationsverzeichnis (default: /opt/tidefly-plane)
  --no-start            Nur konfigurieren, nicht starten
  --with-ui             Dashboard UI Container mitstarten (profile: dashboard)
  --non-interactive     Kein Stdin (für TUI/CI)
  --skip-existing       Existierende .env nicht überschreiben
  -h, --help            Diese Hilfe
EOF
}

# =============================================================================
# Dependency Checks
# =============================================================================
check_deps() {
  log_step "Abhängigkeiten prüfen"
  local missing=()

  for cmd in docker openssl curl; do
    if ! command -v "$cmd" &>/dev/null; then
      missing+=("$cmd")
    fi
  done

  if ! docker compose version &>/dev/null 2>&1; then
    missing+=("docker compose (v2 plugin)")
  fi

  if [[ ${#missing[@]} -gt 0 ]]; then
    log_error "Fehlende Abhängigkeiten: ${missing[*]}"
    exit 2
  fi

  if ! docker info &>/dev/null 2>&1; then
    log_error "Docker Daemon ist nicht erreichbar. Ist Docker gestartet?"
    exit 2
  fi

  log_success "Alle Abhängigkeiten vorhanden"
}

# =============================================================================
# Verzeichnis anlegen
# =============================================================================
setup_dir() {
  log_step "Verzeichnis vorbereiten: $TIDEFLY_DIR"
  mkdir -p "$TIDEFLY_DIR" "$REDIS_DIR"

  if [[ $EUID -eq 0 ]]; then
    chmod 750 "$TIDEFLY_DIR"
  fi

  log_success "Verzeichnis bereit"
}

# =============================================================================
# .env generieren
# =============================================================================
generate_env() {
  log_step ".env generieren"

  if [[ -f "$ENV_FILE" ]]; then
    if [[ "$OPT_SKIP_EXISTING" == "true" ]]; then
      log_warn ".env existiert bereits — überspringe (--skip-existing)"
      return 0
    fi

    if [[ "$OPT_NON_INTERACTIVE" == "false" ]]; then
      echo -e "${YELLOW}⚠ $ENV_FILE existiert bereits.${RESET}"
      read -rp "Überschreiben? Bestehende Secrets gehen verloren. [y/N] " confirm
      [[ "$confirm" =~ ^[Yy]$ ]] || { log_info "Abgebrochen."; exit 3; }
    else
      log_warn ".env existiert — im non-interactive Modus wird übersprungen"
      return 0
    fi
  fi

  local app_secret jwt_secret enc_key pg_pass redis_pass
  app_secret=$(openssl rand -hex 32)
  jwt_secret=$(openssl rand -hex 32)
  enc_key=$(openssl rand -base64 32)
  pg_pass=$(openssl rand -base64 48 | tr -d '=/+' | head -c 48)
  redis_pass=$(openssl rand -base64 48 | tr -d '=/+' | head -c 48)

  cat > "$ENV_FILE" <<EOF
# Generated by tidefly install.sh — $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# DO NOT COMMIT THIS FILE

# ── App ──────────────────────────────────────────────────────────────
APP_ENV=production
APP_PORT=8181
APP_SECRET_KEY=${app_secret}
API_DOCS_ENABLED=false
TIDEFLY_ENCRYPTION_KEY=${enc_key}
AGENT_GRPC_PORT=7443

# ── Database ─────────────────────────────────────────────────────────
DATABASE_URL=postgres://tidefly:${pg_pass}@postgres:5432/tidefly?sslmode=disable
POSTGRES_USER=tidefly
POSTGRES_PASSWORD=${pg_pass}
POSTGRES_DB=tidefly

# ── Redis ────────────────────────────────────────────────────────────
REDIS_URL=redis://tidefly:${redis_pass}@redis:6379/0
REDIS_ADDR=redis:6379
REDIS_USER=tidefly
REDIS_PASSWORD=${redis_pass}

# ── Auth ─────────────────────────────────────────────────────────────
JWT_SECRET=${jwt_secret}

# ── Docker Runtime ───────────────────────────────────────────────────
RUNTIME_TYPE=docker
DOCKER_SOCK=/var/run/docker.sock
RUNTIME_SOCKET=/var/run/docker.sock

# ── SMTP (optional) ──────────────────────────────────────────────────
SMTP_HOST=smtp.example.com
SMTP_PORT=465
SMTP_USER=
SMTP_PASSWORD=
SMTP_FROM=noreply@example.com
SMTP_TLS=tls

# ── Templates ────────────────────────────────────────────────────────
TEMPLATES_DIR=https://github.com/tidefly-oss/tidefly-templates

# ── Caddy ────────────────────────────────────────────────────────────
CADDY_ENABLED=true
CADDY_ADMIN_URL=http://caddy:2019
CADDY_BASE_DOMAIN=apps.example.com
CADDY_ACME_EMAIL=admin@example.com
CADDY_ACME_STAGING=false
CADDY_FORCE_HTTPS=true
CADDY_INTERNAL_TLS=true

# ── Logging ──────────────────────────────────────────────────────────
LOG_LEVEL=info
LOG_DB_LEVEL=warn
LOG_SLOW_QUERY_MS=500

# ── Log Watcher ──────────────────────────────────────────────────────
LOGWATCHER_ENABLED=true
LOGWATCHER_POLL_INTERVAL=15
LOGWATCHER_TAIL_LINES=50
LOGWATCHER_MAX_MESSAGE_LEN=3000
LOGWATCHER_DEDUP_WINDOW=2

# ── Jobs ─────────────────────────────────────────────────────────────
JOBS_ENABLED=true
JOBS_CLEANUP_CRON='0 3 * * *'
JOBS_CLEANUP_OLDER_THAN=24h
JOBS_CLEANUP_STOPPED_CONTAINERS=true
JOBS_CLEANUP_DANGLING_IMAGES=true
JOBS_CLEANUP_UNUSED_VOLUMES=false
JOBS_LOG_RETENTION_CRON='0 4 * * *'
JOBS_LOG_RETENTION_DAYS=30
JOBS_AUDIT_RETENTION_DAYS=90
JOBS_NOTIFICATION_RETENTION_DAYS=30
JOBS_HEALTH_CHECK_CRON='*/5 * * * *'
JOBS_CONCURRENCY=5
EOF

  chmod 600 "$ENV_FILE"

  cat > "$REDIS_DIR/users.acl" <<EOF
user default off
user tidefly on >${redis_pass} ~* &* +@all
EOF
  chmod 600 "$REDIS_DIR/users.acl"

  log_success ".env und Redis ACL generiert"
}

# =============================================================================
# Redis config schreiben
# =============================================================================
setup_redis_conf() {
  local conf="$REDIS_DIR/redis.conf"
  if [[ -f "$conf" ]]; then
    return 0
  fi

  cat > "$conf" <<EOF
# Tidefly Redis config — generated by install.sh
aclfile /usr/local/etc/redis/users.acl
save 900 1
save 300 10
save 60 10000
appendonly yes
appendfilename "appendonly.aof"
dir /data
loglevel notice
EOF
  log_success "redis.conf geschrieben"
}

# =============================================================================
# docker-compose.yaml schreiben
# =============================================================================
write_compose() {
  log_step "docker-compose.yaml schreiben"

  local img_tag="$TIDEFLY_VERSION"

  cat > "$COMPOSE_FILE" <<EOF
# Generated by tidefly install.sh — $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# Tidefly Version: ${img_tag}

networks:
  tidefly_internal:
    driver: bridge
    labels:
      tidefly.internal: "true"
    ipam:
      config:
        - subnet: 172.30.0.0/24
  tidefly_proxy:
    external: true

volumes:
  postgres_data:
  redis_data:
  caddy_data:

services:
  # ── Caddy ──────────────────────────────────────────────────────────
  caddy:
    image: ${TIDEFLY_REPO}/tidefly-caddy:${img_tag}
    container_name: tidefly_caddy
    labels:
      tidefly.internal: "true"
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - caddy_data:/data
    networks:
      - tidefly_proxy
    environment:
      - CADDY_ADMIN=0.0.0.0:2019
    command: caddy run --resume
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:2019/config/"]
      interval: 5s
      timeout: 3s
      retries: 5
      start_period: 5s

  # ── Backend ────────────────────────────────────────────────────────
  backend:
    image: ${TIDEFLY_REPO}/tidefly-plane:${img_tag}
    container_name: tidefly_backend
    labels:
      tidefly.internal: "true"
    restart: unless-stopped
    env_file:
      - ./.env
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - tidefly_internal
      - tidefly_proxy
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      caddy:
        condition: service_healthy

  # ── UI (optional) ─────────────────────────────────────────────────
  ui:
    image: ${TIDEFLY_REPO}/tidefly-ui:${img_tag}
    container_name: tidefly_ui
    labels:
      tidefly.internal: "true"
    restart: unless-stopped
    env_file:
      - ./.env
    networks:
      - tidefly_internal
    depends_on:
      - backend
    profiles:
      - dashboard

  # ── PostgreSQL ────────────────────────────────────────────────────
  postgres:
    image: postgres:17-alpine
    container_name: tidefly_postgres
    labels:
      tidefly.internal: "true"
    restart: unless-stopped
    env_file:
      - ./.env
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - tidefly_internal
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U \${POSTGRES_USER:-tidefly} -d \${POSTGRES_DB:-tidefly}"]
      interval: 5s
      timeout: 5s
      retries: 5
      start_period: 10s

  # ── Redis ─────────────────────────────────────────────────────────
  redis:
    image: redis:8-alpine
    container_name: tidefly_redis
    labels:
      tidefly.internal: "true"
    restart: unless-stopped
    command: ["redis-server", "/usr/local/etc/redis/redis.conf"]
    env_file:
      - ./.env
    volumes:
      - redis_data:/data
      - ./redis:/usr/local/etc/redis
    networks:
      - tidefly_internal
    healthcheck:
      test: ["CMD", "redis-cli", "--user", "tidefly", "--pass", "\${REDIS_PASSWORD}", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5
      start_period: 10s
EOF

  log_success "docker-compose.yaml geschrieben"
}

# =============================================================================
# tidefly_proxy Netzwerk anlegen
# =============================================================================
ensure_proxy_network() {
  if ! docker network inspect tidefly_proxy &>/dev/null 2>&1; then
    log_info "Erstelle tidefly_proxy Netzwerk..."
    docker network create \
      --driver bridge \
      --label tidefly-plane.proxy=true \
      tidefly_proxy
    log_success "tidefly_proxy Netzwerk erstellt"
  else
    log_info "tidefly_proxy Netzwerk existiert bereits"
  fi
}

# =============================================================================
# Images pullen
# =============================================================================
pull_images() {
  log_step "Docker Images pullen (Version: $TIDEFLY_VERSION)"

  local images=(
    "${TIDEFLY_REPO}/tidefly-plane:${TIDEFLY_VERSION}"
    "${TIDEFLY_REPO}/tidefly-caddy:${TIDEFLY_VERSION}"
    "postgres:17-alpine"
    "redis:8-alpine"
  )

  if [[ "$OPT_WITH_UI" == "true" ]]; then
    images+=("${TIDEFLY_REPO}/tidefly-ui:${TIDEFLY_VERSION}")
  fi

  for img in "${images[@]}"; do
    log_info "Pulling $img..."
    docker pull "$img"
  done

  log_success "Alle Images geladen"
}

# =============================================================================
# Starten
# =============================================================================
start_services() {
  log_step "Services starten"

  local compose_args=("--project-directory" "$TIDEFLY_DIR" "-f" "$COMPOSE_FILE")
  local up_args=("-d" "--remove-orphans")

  if [[ "$OPT_WITH_UI" == "true" ]]; then
    compose_args+=("--profile" "dashboard")
  fi

  docker compose "${compose_args[@]}" up "${up_args[@]}"

  log_success "Services gestartet"
}

# =============================================================================
# Health Check
# =============================================================================
wait_for_backend() {
  log_step "Warte auf Backend..."
  local port
  port=$(grep -E '^APP_PORT=' "$ENV_FILE" | cut -d= -f2 || echo "8181")
  local url="http://localhost:${port}/health"
  local max_attempts=30
  local attempt=0

  while [[ $attempt -lt $max_attempts ]]; do
    if curl -sf "$url" &>/dev/null; then
      log_success "Backend ist bereit ✓"
      return 0
    fi
    attempt=$((attempt + 1))
    echo -n "."
    sleep 2
  done

  echo ""
  log_warn "Backend antwortet noch nicht — prüfe: docker compose -f $COMPOSE_FILE logs backend"
}

# =============================================================================
# Zusammenfassung
# =============================================================================
print_summary() {
  local port
  port=$(grep -E '^APP_PORT=' "$ENV_FILE" | cut -d= -f2 || echo "8181")

  echo ""
  echo -e "${GREEN}${BOLD}╔══════════════════════════════════════════════╗${RESET}"
  echo -e "${GREEN}${BOLD}║        Tidefly erfolgreich installiert!      ║${RESET}"
  echo -e "${GREEN}${BOLD}╚══════════════════════════════════════════════╝${RESET}"
  echo ""
  echo -e "  📁 Installationsverzeichnis:  ${BOLD}$TIDEFLY_DIR${RESET}"
  echo -e "  🔧 Version:                   ${BOLD}$TIDEFLY_VERSION${RESET}"
  echo -e "  🌐 Backend API:               ${BOLD}http://localhost:${port}${RESET}"
  echo ""
  echo -e "  Nächste Schritte:"
  echo -e "  1. ${YELLOW}$ENV_FILE${RESET} anpassen (Domain, SMTP, etc.)"
  echo -e "  2. Services neu starten: ${BOLD}docker compose -f $COMPOSE_FILE up -d${RESET}"
  echo ""
  echo -e "  Logs: ${BOLD}docker compose -f $COMPOSE_FILE logs -f${RESET}"
  echo ""
}

# =============================================================================
# Main
# =============================================================================
main() {
  parse_args "$@"

  echo ""
  echo -e "${BOLD}Tidefly Plane Installer${RESET}"
  echo -e "Version: ${TIDEFLY_VERSION}  |  Dir: ${TIDEFLY_DIR}"
  echo ""

  check_deps
  setup_dir
  generate_env
  setup_redis_conf
  write_compose
  ensure_proxy_network
  pull_images

  if [[ "$OPT_NO_START" == "false" ]]; then
    start_services
    wait_for_backend
  else
    log_info "--no-start gesetzt: Services werden nicht gestartet"
  fi

  print_summary
}

main "$@"
