#!/usr/bin/env bash
# =============================================================================
# init-env.sh — Generate clean .env for development or production
# Secrets and URLs always filled, no comments
# Non-interactive: ENV_TYPE must be set externally (TUI or export)
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# ── Choose environment ─────────────────────────────
# ENV_TYPE muss von außen gesetzt werden, z.B.:
#   ENV_TYPE=development ./scripts/init-env.sh
#   ENV_TYPE=production  ./scripts/init-env.sh
ENV_TYPE="${ENV_TYPE:-development}"  # Default fallback: development

ENV_DIR="$SCRIPT_DIR/deploy/$ENV_TYPE"
ENV_FILE="$ENV_DIR/.env"
mkdir -p "$ENV_DIR"

# ── Generate secrets ─────────────────────────────
APP_SECRET_KEY=$(openssl rand -hex 32)
SESSION_SECRET=$(openssl rand -base64 48 | tr -d '=/+' | head -c 48)
COOKIE_SECRET=$(openssl rand -base64 48 | tr -d '=/+' | head -c 48)
POSTGRES_PASSWORD=$(openssl rand -base64 48 | tr -d '=/+' | head -c 48)
REDIS_PASSWORD=$(openssl rand -base64 48 | tr -d '=/+' | head -c 48)

# ── URLs / Flags ─────────────────────────────
if [[ "$ENV_TYPE" == "production" ]]; then
  DATABASE_URL="postgres://tidefly:$POSTGRES_PASSWORD@postgres:5432/tidefly?sslmode=disable"
  REDIS_URL="redis://tidefly:$REDIS_PASSWORD@redis:6379/0"
  REDIS_ADDR="redis:6379"
  TEMPLATES_DIR="https://github.com/tidefly-oss/tidefly-templates"
  TRAEFIK_DASHBOARD_ENABLED="false"
  API_DOCS_ENABLED="false"
else
  DATABASE_URL="postgres://tidefly:$POSTGRES_PASSWORD@127.0.0.1:5432/tidefly?sslmode=disable"
  REDIS_URL="redis://tidefly:$REDIS_PASSWORD@127.0.0.1:6379/0"
  REDIS_ADDR="127.0.0.1:6379"
  TEMPLATES_DIR="../tidefly-templates"
  TRAEFIK_DASHBOARD_ENABLED="true"
  API_DOCS_ENABLED="true"
fi

# ── Write clean .env ─────────────────────────────
cat > "$ENV_FILE" <<EOF
APP_ENV=$ENV_TYPE
APP_PORT=8181
APP_SECRET_KEY=$APP_SECRET_KEY
API_DOCS_ENABLED=$API_DOCS_ENABLED

DATABASE_URL=$DATABASE_URL
POSTGRES_USER=tidefly
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=tidefly

REDIS_URL=$REDIS_URL
REDIS_ADDR=$REDIS_ADDR
REDIS_USER=tidefly
REDIS_PASSWORD=$REDIS_PASSWORD

SESSION_SECRET=$SESSION_SECRET
COOKIE_SECRET=$COOKIE_SECRET

RUNTIME_TYPE=docker
DOCKER_SOCK=/var/run/docker.sock
RUNTIME_SOCKET=/var/run/docker.sock

SMTP_HOST=smtp.example.com
SMTP_PORT=465
SMTP_USER=
SMTP_PASSWORD=
SMTP_FROM=noreply@example.com
SMTP_TLS=tls

TEMPLATES_DIR=$TEMPLATES_DIR

TRAEFIK_ENABLED=true
TRAEFIK_DASHBOARD_ENABLED=$TRAEFIK_DASHBOARD_ENABLED
TRAEFIK_BASE_DOMAIN=apps.example.com
TRAEFIK_ACME_EMAIL=admin@example.com
TRAEFIK_ACME_STAGING=false
TRAEFIK_NETWORK=tidefly_internal
TRAEFIK_FORCE_HTTPS=true
TRAEFIK_LOG_LEVEL=INFO

LOG_LEVEL=info
LOG_DB_LEVEL=warn
LOG_SLOW_QUERY_MS=500

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
JOBS_METRICS_RETENTION_DAYS=30
JOBS_HEALTH_CHECK_CRON='*/5 * * * *'
JOBS_CONCURRENCY=5
EOF

# ── Create Redis ACL file ─────────────────────────────
REDIS_DIR="$ENV_DIR/redis"
mkdir -p "$REDIS_DIR"

# Create users.acl with the generated password
cat > "$REDIS_DIR/users.acl" <<EOF
user default off
user tidefly on >$REDIS_PASSWORD ~* &* +@all
EOF

echo "✅ Clean .env generated for $ENV_TYPE environment with all secrets and URLs!"
echo "✅ Redis ACL file created at $REDIS_DIR/users.acl"
