#!/usr/bin/env bash
# mailezctl — management entry: dev / community / enterprise / ha.
#
# Three self-contained compose files:
#   dev        = docker-compose.dev.yml        (SQLite + Pebble + local FS)
#   community  = docker-compose.community.yml  (mailezine + SQLite control plane)
#   enterprise = docker-compose.enterprise.yml (mailezine + MySQL + TiDB + MinIO/S3)
# plus the HA overlay:
#   ha         = enterprise + docker-compose.ha.yml (backend/frontend replicas)
#
# Usage:
#   ./deploy/mailezctl.sh up              # dev
#   ./deploy/mailezctl.sh up community    # community edition prod
#   ./deploy/mailezctl.sh up enterprise   # enterprise edition prod
#   ./deploy/mailezctl.sh up ha           # enterprise + control-plane replicas
#   ./deploy/mailezctl.sh ps
#   ./deploy/mailezctl.sh logs enterprise mailezine -Follow
#   ./deploy/mailezctl.sh down
set -euo pipefail

ACTION="${1:-ps}"
TARGET="${2:-dev}"
SERVICE="${3:-}"

cd "$(dirname "$0")"

case "$TARGET" in
  dev)        FILES=("docker-compose.dev.yml") ;;
  community)  FILES=("docker-compose.community.yml") ;;
  enterprise) FILES=("docker-compose.enterprise.yml") ;;
  ha)         FILES=("docker-compose.enterprise.yml" "docker-compose.ha.yml") ;;
  *)
    echo "unknown target: $TARGET (dev|community|enterprise|ha)" >&2
    exit 2
    ;;
esac

ARGS=(--env-file mailez.env)
for f in "${FILES[@]}"; do ARGS+=(-f "$f"); done
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
