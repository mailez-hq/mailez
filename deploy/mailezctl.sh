#!/usr/bin/env bash
# mailezctl — unified orchestration entry (Linux/macOS).
#
# One command manages the whole stack (mailez control plane + mail engine);
# engine/storage is selected via compose profiles inside one "mailez" project:
#
# Storage matrix (dev = SQLite + Pebble/TiDB + local FS; prod = MySQL +
# TiDB + MinIO/S3):
#
#   Engine          Dev base                      Prod (-Prod)
#   --------------- ----------------------------  --------------------------------------------
#   postdove        docker-compose.dev.yml        compose + mysql
#   mailezine       dev + mailezine (Pebble+FS)    compose + mailezine + blob-s3 + mysql
#   mailezine-tidb  dev + mailezine + tidb         compose + mailezine + tidb + blob-s3 + mysql
#
# Usage:
#   ./deploy/mailezctl.sh up mailezine-tidb
#   ./deploy/mailezctl.sh up mailezine
#   ./deploy/mailezctl.sh ps
#   ./deploy/mailezctl.sh logs mailezine -Follow
#   ./deploy/mailezctl.sh down
set -euo pipefail

ACTION="${1:-ps}"
ENGINE="${2:-postdove}"
SERVICE="${3:-}"
PROD="${MAILEZCTL_PROD:-0}"

cd "$(dirname "$0")"

BASE="docker-compose.dev.yml"
[ "$PROD" = "1" ] && BASE="docker-compose.yml"

FILES=(-f "$BASE")
PROFILE=""
case "$ENGINE" in
  mailezine)
    FILES+=(-f docker-compose.mailezine.yml)
    PROFILE="mailezine"
    ;;
  mailezine-tidb)
    FILES+=(-f docker-compose.mailezine.yml -f docker-compose.tidb.yml)
    PROFILE="mailezine"
    ;;
  postdove) ;;
  *) 
    echo "unknown engine: $ENGINE (postdove|mailezine|mailezine-tidb)" >&2
    exit 2
    ;;
esac
if [ "$PROD" = "1" ]; then
  FILES+=(-f docker-compose.mysql.yml)
  if [ "$ENGINE" != "postdove" ]; then
    FILES+=(-f docker-compose.blob-s3.yml)
  fi
fi

ARGS=()
if [ -n "$PROFILE" ]; then ARGS+=(--profile "$PROFILE"); fi

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

echo "== mailezctl $ACTION (engine=$ENGINE, prod=$PROD)"
exec docker compose "${FILES[@]}" "${ARGS[@]}"
