#!/usr/bin/env bash
# mailezctl — management entry: dev / prod (+ optional postfix+dovecot).
#
# Default tiers:
#   dev  = docker-compose.dev.yml   (SQLite + Pebble + local FS)
#   prod = docker-compose.prod.yml  (MySQL + TiDB + MinIO/S3)
# Optional classic engine (self-contained, maildir storage):
#   postdove = docker-compose.postdove.yml (MySQL control plane + postfix/dovecot)
#
# Usage:
#   ./deploy/mailezctl.sh up              # dev
#   ./deploy/mailezctl.sh up prod         # prod
#   ./deploy/mailezctl.sh up postdove     # postfix+dovecot prod
#   ./deploy/mailezctl.sh ps
#   ./deploy/mailezctl.sh logs prod mailezine -Follow
#   ./deploy/mailezctl.sh down
set -euo pipefail

ACTION="${1:-ps}"
TARGET="${2:-dev}"
SERVICE="${3:-}"

cd "$(dirname "$0")"

case "$TARGET" in
  dev)      FILE="docker-compose.dev.yml" ;;
  prod)     FILE="docker-compose.prod.yml" ;;
  postdove) FILE="docker-compose.postdove.yml" ;;
  *)
    echo "unknown target: $TARGET (dev|prod|postdove)" >&2
    exit 2
    ;;
esac

ARGS=(-f "$FILE")
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
