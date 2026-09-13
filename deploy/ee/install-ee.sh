#!/usr/bin/env bash
# install-ee: enterprise offline installer. Run inside the unpacked release
# bundle (mailez-ee-<version>/) on the customer's Linux server, as root:
#
#   sudo bash install-ee.sh                       # interactive
#   sudo bash install-ee.sh --domain mailez.cn --admin-password '...' --yes
#   sudo bash install-ee.sh activate /path/license.lic [/path/service.lic]
#
# Flow: preflight checks -> docker load images -> generate mailez.env with
# fresh secrets (license NOT required yet) -> compose up -> wait healthy ->
# seed the admin account -> print access info and DNS guidance.
# "activate" installs a paid license afterwards and switches enforcement on.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
VERSION="$(cat "${HERE}/VERSION" 2>/dev/null || echo unknown)"
INSTALL_DIR="${INSTALL_DIR:-/opt/mailez}"
STACK_NAME="mailez"

say()  { printf '\e[1;34m==>\e[0m %s\n' "$*"; }
warn() { printf '\e[1;33m ->\e[0m %s\n' "$*"; }
die()  { printf '\e[1;31mERROR:\e[0m %s\n' "$*" >&2; exit 1; }

preflight() {
  [ "$(id -u)" = 0 ] || die "run as root (sudo)"
  command -v docker >/dev/null || die "docker not installed (https://docs.docker.com/engine/install/)"
  docker compose version >/dev/null 2>&1 || die "docker compose v2 plugin missing"
  MEM_MB="$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo)"
  [ "${MEM_MB}" -ge 7000 ] || warn "memory ${MEM_MB}MB < 8GB recommended for 500 users"
  AVAIL_GB="$(df -BG --output=avail "$(dirname "${INSTALL_DIR}")" | tail -1 | tr -dc '0-9')"
  [ "${AVAIL_GB:-0}" -ge 40 ] || warn "free disk ${AVAIL_GB}GB < 40GB recommended"
  for p in 25 80 443 8081 8082 8083 465 587 143 993; do
    ss -ltn "sport = :${p}" 2>/dev/null | grep -q LISTEN && warn "port ${p} already in use — mail/web ports must be free"
  done
  command -v openssl >/dev/null || die "openssl not found"
}

ask() { # ask VAR PROMPT DEFAULT
  local __var="$1" __prompt="$2" __def="$3" __cur
  __cur="$(eval "printf '%s' \"\${$1:-}\"")"
  if [ "${ASSUME_YES}" = 1 ]; then
    # Non-interactive: an explicitly passed value ALWAYS wins (the early
    # version fell through to read when the default was empty — SSH has no
    # stdin, so --admin-password silently became a random password and the
    # printed credentials diverged from what the operator requested).
    if [ -n "${__cur}" ]; then eval "$1=\"\${__cur}\""; return; fi
    if [ -n "${__def}" ]; then eval "$1=\"\${__def}\""; return; fi
  fi
  printf '%s' "${__prompt} [${__def}]: "
  read -r answer
  eval "$1=\"\${answer:-\${__def}}\""
}

install_stack() {
  preflight

  if [ -f "${INSTALL_DIR}/mailez.env" ]; then
    die "${INSTALL_DIR}/mailez.env already exists — refusing to overwrite. To reinstall, move it away first (data under ${INSTALL_DIR}/data is preserved)."
  fi

  # Non-interactive overrides first, interactive defaults after.
  ASSUME_YES="${ASSUME_YES:-0}"
  ask MAILEZ_DOMAIN    "mail domain (e.g. mailez.cn)" ""
  ask MAILEZ_HOSTNAME  "public mail hostname" "mail.${MAILEZ_DOMAIN}"
  ask MAILEZ_ADMIN_EMAIL "admin account email" "admin@${MAILEZ_DOMAIN}"
  ask MAILEZ_ADMIN_PASSWORD "admin password (empty = random)" ""
  [ -n "${MAILEZ_DOMAIN}" ] || die "mail domain is required"
  [ -n "${MAILEZ_ADMIN_PASSWORD}" ] || MAILEZ_ADMIN_PASSWORD="$(openssl rand -base64 15 | tr -d '/+=' | cut -c1-14)"
  ask INSTALL_DIR      "install directory" "/opt/mailez"

  say "loading images (this takes a few minutes)"
  docker load -i "${HERE}/images.tar.gz"

  say "preparing ${INSTALL_DIR}"
  mkdir -p "${INSTALL_DIR}/ee" "${INSTALL_DIR}/certs" "${INSTALL_DIR}/licenses"
  cp "${HERE}/ee/docker-compose.ee.yml" "${INSTALL_DIR}/ee/"
  # The compose file references ../data, ../certs, ../licenses and
  # ../mailez.env relative to ee/ — pre-create data with the uid:gid the
  # containers expect (backend 1000, engine 82).
  mkdir -p "${INSTALL_DIR}/data"
  chown -R 1000:82 "${INSTALL_DIR}/data" 2>/dev/null || warn "could not chown data dir (rootless docker?)"
  # The engine (82:101) needs its own mount writable, otherwise it comes up
  # with full-text search disabled ("mkdir /data/rocks.fts: permission
  # denied") because Docker created the subdirectory as root.
  mkdir -p "${INSTALL_DIR}/data/mailezine"
  chown -R 82:101 "${INSTALL_DIR}/data/mailezine" 2>/dev/null || warn "could not chown engine data dir"

  say "generating ${INSTALL_DIR}/mailez.env"
  {
    echo "# mailez enterprise deployment — generated $(date -u +%FT%TZ)"
    echo "MAILEZ_DOMAIN=${MAILEZ_DOMAIN}"
    echo "MAILEZ_HOSTNAME=${MAILEZ_HOSTNAME}"
    echo "MAILEZ_SECRET_KEY=$(openssl rand -hex 16)"
    echo "MAILEZ_STACK_SECRET=$(openssl rand -hex 32)"
    echo "MAILEZINE_STACK_SECRET=\${MAILEZ_STACK_SECRET}"
    echo "MAILEZ_IMAGE_TAG=${VERSION}"
    # Gateway-family services (gateway/resolver/mail-filter) receive their
    # environment ONLY from this file, and the agent refuses to start on an
    # empty MAILEZ_HOSTNAMES/MAILEZ_SUBNET — the gap burned the first real
    # deployment. Keep every reference-env key that has no compose-level
    # default.
    echo "MAILEZ_HOSTNAMES=${MAILEZ_HOSTNAME}"
    # The engine greets remote MTAs with MAILEZINE_HOSTNAME; without it the
    # banner (and EHLO) degrades to the container id, which receiving MTAs
    # and spam filters score against.
    echo "MAILEZINE_HOSTNAME=${MAILEZ_HOSTNAME}"
    echo "MAILEZ_SUBNET=192.168.206.0/24"
    echo "MAILEZ_SUBNET6=fd00:beef:beef::/64"
    # Bare local part: the agent templates append @${MAILEZ_DOMAIN} themselves
    # (rspamd dmarc.conf, ACME account contact). A full address here produced
    # admin@mailez.cn@mailez.cn and every first TLS enable failed at Let's
    # Encrypt registration with invalidContact.
    echo "MAILEZ_POSTMASTER=postmaster"
    echo "MAILEZ_RELAYNETS="
    echo "MAILEZ_TLS=off"
    # 443 must stay in this list: it is what makes the gateway render the
    # HTTPS vhost after ACME succeeds (25/465/587 are engine-side).
    echo "MAILEZ_TLS_PORTS=80,443"
    echo "MAILEZ_ANTIVIRUS=none"
    echo "MAILEZ_SCAN_MACROS=true"
    echo "MAILEZ_GREYLISTING=false"
    echo "MAILEZ_MESSAGE_SIZE_LIMIT=52428800"
    echo "MAIL_SIEVE_ADDR=mailezine:4190"
    echo "TZ=Asia/Shanghai"
    # Trial phase: the stack runs without a license. activate turns this on.
    echo "MAILEZINE_LICENSE_REQUIRED=false"
    echo "MAILEZ_ADMIN_EMAIL=${MAILEZ_ADMIN_EMAIL}"
    echo "MAILEZ_ADMIN_PASSWORD=${MAILEZ_ADMIN_PASSWORD}"
  } > "${INSTALL_DIR}/mailez.env"
  chmod 600 "${INSTALL_DIR}/mailez.env"

  say "starting the stack"
  cd "${INSTALL_DIR}"
  docker compose --env-file mailez.env -p "${STACK_NAME}" -f ee/docker-compose.ee.yml up -d

  say "waiting for the backend to become healthy"
  for i in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:8081/api/v1/health" >/dev/null 2>&1; then break; fi
    [ "$i" = 60 ] && die "backend did not become healthy; check: docker compose -p ${STACK_NAME} logs backend"
    sleep 5
  done

  say "seeding the admin account"
  docker compose --env-file mailez.env -p "${STACK_NAME}" -f ee/docker-compose.ee.yml exec -T backend mailez-seed

  cat <<SUMMARY

========================================================================
 mailez enterprise ${VERSION} is up.

 Admin console : http://<this-host>       (gateway :80; 127.0.0.1:8082 for local debug)
 Webmail       : http://<this-host>       (gateway :80; 127.0.0.1:8083 for local debug)
 Admin account : ${MAILEZ_ADMIN_EMAIL}
 Admin password: ${MAILEZ_ADMIN_PASSWORD}
 Deploy dir    : ${INSTALL_DIR}   (data: ${INSTALL_DIR}/data)

 DNS — publish these at your provider now (the admin console has a
 per-domain wizard with the exact records and live verification:
 Domains -> DNS):
   MX ${MAILEZ_DOMAIN} -> ${MAILEZ_HOSTNAME}
   TXT ${MAILEZ_DOMAIN} -> "v=spf1 mx -all"
   TXT _dmarc.${MAILEZ_DOMAIN} -> "v=DMARC1; p=quarantine"

 Licensing — this deployment runs unlicensed for now. After purchase:
   sudo bash ${HERE}/install-ee.sh activate license.lic [service.lic]

 TLS — off by default; see README-INSTALL-EE.md for production TLS.
========================================================================
SUMMARY
}

activate_license() {
  [ "$(id -u)" = 0 ] || die "run as root (sudo)"
  [ -f "${INSTALL_DIR}/mailez.env" ] || die "no deployment found at ${INSTALL_DIR}"
  LIC="${1:-}"; SVC="${2:-}"
  [ -n "${LIC}" ] && [ -f "${LIC}" ] || die "usage: install-ee.sh activate <license.lic> [service.lic]"
  [ -s "${LIC}" ] || die "license file is empty: ${LIC}"

  say "installing license"
  install -m 0644 "${LIC}" "${INSTALL_DIR}/licenses/license.lic"
  [ -n "${SVC}" ] && [ -f "${SVC}" ] && install -m 0644 "${SVC}" "${INSTALL_DIR}/licenses/service.lic"

  say "switching license enforcement on"
  sed -i.bak 's/^MAILEZINE_LICENSE_REQUIRED=.*/MAILEZINE_LICENSE_REQUIRED=true/' "${INSTALL_DIR}/mailez.env"

  cd "${INSTALL_DIR}"
  docker compose --env-file mailez.env -p "${STACK_NAME}" -f ee/docker-compose.ee.yml up -d

  say "waiting for the backend to become healthy"
  for i in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:8081/api/v1/health" >/dev/null 2>&1; then break; fi
    [ "$i" = 60 ] && die "backend did not become healthy after activation — check the license file and: docker compose -p ${STACK_NAME} logs backend mailezine"
    sleep 5
  done

  cat <<DONE

========================================================================
 License activated. Verify in the admin console (Overview):
   edition, licensee and the mailbox cap are shown there, and creating
   accounts beyond the cap is refused from now on.
========================================================================
DONE
}

case "${1:-}" in
  activate) shift; activate_license "$@" ;;
  "") install_stack ;;
  *) 
    # Flags (--domain x --hostname y --admin-email a --admin-password p
    # --install-dir d --yes) are folded into env vars, then install.
    while [ $# -gt 0 ]; do
      case "$1" in
        --domain) MAILEZ_DOMAIN="$2"; shift 2 ;;
        --hostname) MAILEZ_HOSTNAME="$2"; shift 2 ;;
        --admin-email) MAILEZ_ADMIN_EMAIL="$2"; shift 2 ;;
        --admin-password) MAILEZ_ADMIN_PASSWORD="$2"; shift 2 ;;
        --install-dir) INSTALL_DIR="$2"; shift 2 ;;
        --yes|-y) ASSUME_YES=1; shift ;;
        *) die "unknown option: $1 (usage: install-ee.sh [--domain D] [--hostname H] [--admin-email E] [--admin-password P] [--install-dir DIR] [--yes] | activate LIC [SVC])" ;;
      esac
    done
    install_stack ;;
esac
