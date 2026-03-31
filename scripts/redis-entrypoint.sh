#!/usr/bin/env bash
set -euo pipefail

ENV_TYPE="${ENV_TYPE:-development}"
REDIS_USER="${REDIS_USER:-tidefly}"
REDIS_PASSWORD="${REDIS_PASSWORD:-redis_secret}"

if [[ "$ENV_TYPE" == "production" ]]; then
    ACL_DIR="/usr/local/etc/redis"
else
    ACL_DIR="/usr/local/etc/redis"
fi

ACL_FILE="$ACL_DIR/users.acl"

# ── Validierung ─────────────────────────────────────────────
if [[ -z "$REDIS_PASSWORD" ]]; then
  echo "❌ REDIS_PASSWORD ist not set"
  exit 1
fi

if [[ "$REDIS_PASSWORD" == "redis_secret" ]]; then
  echo "⚠️  WARNUNG: default password used — please change in .env"
fi

# ── ACL-Datei erstellen ─────────────────────────────────────
mkdir -p "$ACL_DIR"
cat > "$ACL_FILE" <<EOF
user default off
user $REDIS_USER on >$REDIS_PASSWORD ~* &* +@all
EOF
chmod 600 "$ACL_FILE"

echo "✅ Redis ACL created for user=$REDIS_USER"

# ── Redis starten ─────────────────────────────────────────
exec redis-server /usr/local/etc/redis/redis.conf --aclfile "$ACL_FILE"