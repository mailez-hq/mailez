#!/usr/bin/env bash
# install.sh — interactive one-key installer for the mailez release tarball.
#
# Run from the extracted release directory:
#   ./install.sh
# Or non-interactively (all defaults):
#   ./install.sh --yes
#
# What it does:
#   1. preflight: docker / compose v2 / daemon / openssl / curl
#   2. asks: mail domain, image tag, host ports
#   3. generates MAILEZ_SECRET_KEY + MAILEZ_STACK_SECRET and writes
#      deploy/mailez.env
#   4. starts the stack (deploy/mailezctl.sh up ce)
#   5. waits for the API to go healthy and seeds the initial admin
#      (admin@example.com / MailezDemo2026! — change it after first login)
set -euo pipefail

CDIR="$(cd "$(dirname "$0")" && pwd)"
cd "$CDIR"

DOMAIN="" TAG="" YES=0 DRY_RUN=0
while [ $# -gt 0 ]; do
  case "$1" in
    --domain)  DOMAIN="$2";  shift 2 ;;
    --tag)     TAG="$2";     shift 2 ;;
    --yes)     YES=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

log()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m WARN\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m FAIL\033[0m %s\n' "$*" >&2; exit 1; }

ask() { # ask <prompt> <default>
  local answer
  read -r -p "$1 [$2]: " answer
  echo "${answer:-$2}"
}

# --- preflight ---------------------------------------------------------------
command -v docker  >/dev/null || die "docker not found: https://docs.docker.com/engine/install/"
docker compose version >/dev/null 2>&1 || die "docker compose v2 missing (docker-compose v1 is not supported)"
docker info >/dev/null 2>&1    || die "docker daemon not reachable (permission? try sudo)"
command -v openssl >/dev/null  || die "openssl not found"
command -v curl    >/dev/null  || die "curl not found"

[ -f deploy/mailezctl.sh ] || die "run this script from the extracted release directory"
[ -d deploy/data ] && die "deploy/data already exists: this host already has a mailez install"

# --- defaults ----------------------------------------------------------------
[ -n "$TAG" ] || TAG="$(head -n1 VERSION 2>/dev/null || echo latest)"
TARGET=ce     # the installer provisions the ce compose profile

if [ "$YES" != 1 ]; then
  echo
  echo "  mailez installer (release $TAG)"
  echo "  -------------------------------------------------"
  [ -n "$DOMAIN" ] || DOMAIN="$(ask "Mail domain (MX points here)" example.com)"
  TAG="$(ask "Image tag" "$TAG")"
  HTTP_PORT="$(ask  "HTTP  port" 80)"
  HTTPS_PORT="$(ask "HTTPS port" 443)"
  API_PORT="$(ask   "API   port" 8081)"
  ADMIN_PORT="$(ask "Admin port" 8082)"
  WEBMAIL_PORT="$(ask "Webmail port" 8083)"
else
  [ -n "$DOMAIN" ] || DOMAIN=example.com
  HTTP_PORT=80; HTTPS_PORT=443; API_PORT=8081; ADMIN_PORT=8082; WEBMAIL_PORT=8083
fi

# --- mailez.env --------------------------------------------------------------
SECRET_KEY="$(openssl rand -hex 16)"
STACK_SECRET="$(openssl rand -hex 32)"
ENV_FILE="deploy/mailez.env"
cp deploy/mailez.env.example "$ENV_FILE"
sed -i.bak \
  -e "s/^MAILEZ_SECRET_KEY=.*/MAILEZ_SECRET_KEY=$SECRET_KEY/" \
  -e "s/^MAILEZ_STACK_SECRET=.*/MAILEZ_STACK_SECRET=$STACK_SECRET/" \
  -e "s/^MAILEZINE_STACK_SECRET=.*/MAILEZINE_STACK_SECRET=$STACK_SECRET/" \
  -e "s/^MAILEZ_DOMAIN=.*/MAILEZ_DOMAIN=$DOMAIN/" \
  -e "s/^MAILEZ_HOSTNAMES=.*/MAILEZ_HOSTNAMES=mail.$DOMAIN/" \
  -e "s/^MAILEZ_IMAGE_TAG=.*/MAILEZ_IMAGE_TAG=$TAG/" \
  "$ENV_FILE" && rm -f "$ENV_FILE.bak"
cat >> "$ENV_FILE" <<EOF

# --- written by install.sh ---
MAILEZ_HTTP_PORT=$HTTP_PORT
MAILEZ_HTTPS_PORT=$HTTPS_PORT
MAILEZ_API_PORT=$API_PORT
MAILEZ_ADMIN_PORT=$ADMIN_PORT
MAILEZ_WEBMAIL_PORT=$WEBMAIL_PORT
EOF

log "wrote $ENV_FILE (domain=$DOMAIN tag=$TAG)"
if [ "$DRY_RUN" = 1 ]; then
  log "dry run: stopping before docker compose up"
  exit 0
fi

# --- boot --------------------------------------------------------------------
log "starting the stack (this pulls the images on first run)"
( cd deploy && ./mailezctl.sh up "$TARGET" )

log "waiting for the API on 127.0.0.1:$API_PORT"
for i in $(seq 1 90); do
  if curl -sf "http://127.0.0.1:$API_PORT/api/v1/health" >/dev/null; then
    break
  fi
  [ "$i" = 90 ] && {
    ( cd deploy && docker compose --env-file mailez.env -f "docker-compose.$TARGET.yml" ps -a )
    die "API did not become healthy; check: (cd deploy && ./mailezctl.sh logs $TARGET backend)"
  }
  sleep 2
done

log "seeding the initial admin account"
( cd deploy && docker compose --env-file mailez.env -f "docker-compose.$TARGET.yml" exec -T backend mailez-seed ) \
  || warn "seed failed (maybe already seeded)"

cat <<EOF

  mailez is up
  -------------------------------------------------
    admin    http://localhost:$ADMIN_PORT   (admin@example.com / MailezDemo2026!)
    webmail  http://localhost:$WEBMAIL_PORT
    api      http://localhost:$API_PORT/api/v1/health

  Mail domain: $DOMAIN  (MX -> this host; firewall: 25/tcp in)
  Upgrade later: edit MAILEZ_IMAGE_TAG in $ENV_FILE, then
    (cd deploy && ./mailezctl.sh up $TARGET)
  Logs: (cd deploy && ./mailezctl.sh logs $TARGET backend)
EOF
