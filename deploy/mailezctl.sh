#!/usr/bin/env bash
# mailezctl — management entry: dev / community / enterprise / ha / multi.
#
# Profiles (self-contained compose files):
#   dev        = docker-compose.dev.yml        (SQLite + Pebble + local FS)
#   community  = docker-compose.community.yml  (mailezine + SQLite control plane)
#   enterprise = docker-compose.enterprise.yml (mailezine + MySQL + TiDB + MinIO/S3)
# plus the overlays:
#   ha         = enterprise + docker-compose.ha.yml (backend/frontend replicas)
#   multi      = enterprise + docker-compose.multi.yml (multi-active engine)
#
# Images:
#   Production targets pull prebuilt images (tag = MAILEZ_IMAGE_TAG,
#   default latest; see mailez.env.example). Set MAILEZ_LOCAL_BUILD=1 to
#   build from source instead — the matching docker-compose.build.*.yml
#   overlay is added and the stack comes up with `up -d --build` using the
#   :local tag (what `docker buildx bake` produces by default). dev always
#   builds from source; it is a developer profile without released images
#   to pull.
#
# Usage:
#   ./deploy/mailezctl.sh up              # dev (builds from source)
#   ./deploy/mailezctl.sh up community    # community edition prod (pull)
#   MAILEZ_LOCAL_BUILD=1 ./deploy/mailezctl.sh up community
#   ./deploy/mailezctl.sh up enterprise   # enterprise edition prod (pull)
#   ./deploy/mailezctl.sh up ha           # enterprise + control-plane replicas
#   ./deploy/mailezctl.sh up multi        # enterprise + multi-active engine
#   ./deploy/mailezctl.sh ps
#   ./deploy/mailezctl.sh logs enterprise mailezine -Follow
#   ./deploy/mailezctl.sh down
set -euo pipefail

ACTION="${1:-ps}"
TARGET="${2:-dev}"
SERVICE="${3:-}"

cd "$(dirname "$0")"

case "$TARGET" in
  dev)        FILES=("docker-compose.dev.yml");        BUILD_OVERLAY="docker-compose.build.dev.yml" ;;
  community)  FILES=("docker-compose.community.yml");  BUILD_OVERLAY="docker-compose.build.community.yml" ;;
  enterprise) FILES=("docker-compose.enterprise.yml"); BUILD_OVERLAY="docker-compose.build.enterprise.yml" ;;
  ha)         FILES=("docker-compose.enterprise.yml" "docker-compose.ha.yml"); BUILD_OVERLAY="docker-compose.build.enterprise.yml" ;;
  multi)      FILES=("docker-compose.enterprise.yml" "docker-compose.multi.yml"); BUILD_OVERLAY="docker-compose.build.multi.yml" ;;
  *)
    echo "unknown target: $TARGET (dev|community|enterprise|ha|multi)" >&2
    exit 2
    ;;
esac

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

# Build overlays only apply to build-relevant actions; down/ps/logs work
# with the base files alone (release tarballs ship without build overlays).
if [ "$LOCAL_BUILD" = "1" ] && [ -f "$BUILD_OVERLAY" ]; then
  case "$ACTION" in
    up|build|config) ARGS+=(-f "$BUILD_OVERLAY") ;;
  esac
fi
if [ "$ACTION" = "up" ] && [ "$LOCAL_BUILD" = "1" ]; then
  ARGS+=(--build)
fi

# -Follow may appear anywhere after the service name (the header examples
# put it last): detect it among the trailing args instead of positionally.
if [ "$ACTION" = "logs" ]; then
  ARGS+=("--follow")
fi
if [ -n "$SERVICE" ]; then ARGS+=("$SERVICE"); fi

echo "== mailezctl $ACTION ($TARGET)"
exec docker compose "${ARGS[@]}"
