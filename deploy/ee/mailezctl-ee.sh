#!/usr/bin/env bash
# mailezctl-ee — private-tree management entry: dev / ce / ee / ha / multi.
#
# Not exported to the public source tree. The public mailezctl only knows
# the dev / ce profiles; this companion adds the extended deployment
# recipes that live under deploy/ee/:
#   ee    = ee/docker-compose.ee.yml      (mailezine + MySQL + TiDB + MinIO/S3)
#   ha    = ee + ee/docker-compose.ha.yml (control-plane replicas)
#   multi = ee + ee/docker-compose.multi.yml (multi-active engine)
#
# Usage:
#   ./deploy/ee/mailezctl-ee.sh up ee     # production (pull)
#   MAILEZ_LOCAL_BUILD=1 ./deploy/ee/mailezctl-ee.sh up ee
#   ./deploy/ee/mailezctl-ee.sh up ha
#   ./deploy/ee/mailezctl-ee.sh up multi
#   ./deploy/ee/mailezctl-ee.sh ps ee
#   ./deploy/ee/mailezctl-ee.sh logs ee backend -Follow
#   ./deploy/ee/mailezctl-ee.sh down ee
set -euo pipefail

ACTION="${1:-ps}"
TARGET="${2:-ee}"
SERVICE="${3:-}"

cd "$(dirname "$0")/.."

case "$TARGET" in
  dev)   FILES=("docker-compose.dev.yml");  BUILD_OVERLAY="docker-compose.build.dev.yml" ;;
  ce)    FILES=("docker-compose.ce.yml");   BUILD_OVERLAY="docker-compose.build.ce.yml" ;;
  ee)    FILES=("ee/docker-compose.ee.yml");   BUILD_OVERLAY="ee/docker-compose.build.ee.yml" ;;
  ha)    FILES=("ee/docker-compose.ee.yml" "ee/docker-compose.ha.yml"); BUILD_OVERLAY="ee/docker-compose.build.ee.yml" ;;
  multi) FILES=("ee/docker-compose.ee.yml" "ee/docker-compose.multi.yml"); BUILD_OVERLAY="ee/docker-compose.build.multi.yml" ;;
  *)
    echo "unknown target: $TARGET (dev|ce|ee|ha|multi)" >&2
    exit 2
    ;;
esac

for f in "${FILES[@]}"; do
  if [ ! -f "$f" ]; then
    echo "missing compose file: $f (run from the private tree)" >&2
    exit 2
  fi
done

# Local source builds use the :local tag that docker buildx bake produces by
# default. Exported shell env beats the mailez.env entry for compose
# interpolation, so the build never silently pulls the pinned release tag.
LOCAL_BUILD=0
if [ "${MAILEZ_LOCAL_BUILD:-0}" = "1" ] || [ "$TARGET" = "dev" ]; then
  LOCAL_BUILD=1
  if [ -z "${MAILEZ_IMAGE_TAG:-}" ]; then
    export MAILEZ_IMAGE_TAG=local
  fi
fi

ARGS=(--env-file mailez.env)
for f in "${FILES[@]}"; do ARGS+=(-f "$f"); done
case "$ACTION" in
  up)     ARGS+=(up -d) ;;
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

if [ "$LOCAL_BUILD" = "1" ] && [ -f "$BUILD_OVERLAY" ]; then
  case "$ACTION" in
    up|build|config) ARGS+=(-f "$BUILD_OVERLAY") ;;
  esac
fi
if [ "$ACTION" = "up" ] && [ "$LOCAL_BUILD" = "1" ]; then
  ARGS+=(--build)
fi

if [ "$ACTION" = "logs" ]; then
  ARGS+=("--follow")
fi
if [ -n "$SERVICE" ]; then ARGS+=("$SERVICE"); fi

echo "== mailezctl-ee $ACTION ($TARGET)"
exec docker compose "${ARGS[@]}"
