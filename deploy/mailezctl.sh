#!/usr/bin/env bash
# mailezctl — management entry: dev / community / enterprise.
#
# Three self-contained compose files:
#   dev        = docker-compose.dev.yml        (SQLite + Pebble + local FS)
#   community  = docker-compose.community.yml  (mailezine + MySQL)
#   enterprise = docker-compose.enterprise.yml (mailezine + MySQL + TiDB + MinIO/S3)
#
# Usage:
#   ./deploy/mailezctl.sh up              # dev
#   ./deploy/mailezctl.sh up community    # community edition prod
#   ./deploy/mailezctl.sh up enterprise   # enterprise edition prod
#   ./deploy/mailezctl.sh ps
#   ./deploy/mailezctl.sh logs enterprise mailezine -Follow
#   ./deploy/mailezctl.sh down
set -euo pipefail

ACTION="${1:-ps}"
TARGET="${2:-dev}"
SERVICE="${3:-}"

cd "$(dirname "$0")"

case "$TARGET" in
  dev)        FILE="docker-compose.dev.yml" ;;
  community)  FILE="docker-compose.community.yml" ;;
  enterprise) FILE="docker-compose.enterprise.yml" ;;
  *)
    echo "unknown target: $TARGET (dev|community|enterprise)" >&2
    exit 2
    ;;
esac

ARGS=(--env-file mailez.env -f "$FILE")
case "$ACTION" in
  up)     ARGS+=(up -d --build) ;;
  down)   ARGS+=(down) ;;
  ps)     ARGS+=(ps) ;;
  logs)   ARGS+=(logs) ;;
  build)  ARGS+=(build) ;;
  config) ARGS+=(config) ;;
  *)
    echo "unknown action: $ACTION (up|down|ps|logs|build|config)" >&2
    exit 2
    ;;
esac

if [ "$ACTION" = "logs" ] && [ "${3:-}" = "-Follow" ]; then
  ARGS+=(--follow)
  SERVICE="${4:-}"
fi
if [ -n "$SERVICE" ]; then ARGS+=("$SERVICE"); fi

echo "== mailezctl $ACTION ($TARGET)"
exec docker compose "${ARGS[@]}"
