#!/usr/bin/env bash
# Run from the repo root: bash scripts/gen-proto.sh
set -euo pipefail
cd "$(dirname "$0")/.."

PROTO_DIR="internal/api/v1/proto/agent"
OUT_DIR="internal/api/v1/proto/agent"

mkdir -p "$OUT_DIR"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out="$OUT_DIR" \
  --go_opt=paths=source_relative \
  --go-grpc_out="$OUT_DIR" \
  --go-grpc_opt=paths=source_relative \
  "$PROTO_DIR/agent.proto"

echo "✅ Proto generated → $OUT_DIR"