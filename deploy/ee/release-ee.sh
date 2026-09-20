#!/usr/bin/env bash
# release-ee: build the enterprise release images and assemble the offline
# customer bundle (images + locked compose + installer). Run from deploy/:
#
#   MAILEZ_LICENSE_PUBKEY=<base64 DER PKIX> bash ee/release-ee.sh v1.0.0
#   bash ee/release-ee.sh v1.0.0 --pubkey-file prod.pub        # same, from file
#
# With neither set, ee/prod.pub is used when present: the vendor public key
# lives next to the EE tooling (a path the CE export strips), so a clone of
# this private repository can cut a release without exporting the key from
# the secret store, and the public tree never carries it.
#
# The public key MUST be the vendor production key (license genkey output).
# A release without it is refused unless MAILEZ_ALLOW_DEVKEY=1 — dev-key
# images can never enforce MAILEZ_LICENSE_REQUIRED, so shipping one by
# accident gives away the product.
#
# The technical-service certificate (service.lic) is verified with the same
# keypair, so MAILEZ_SERVICE_PUBKEY defaults to MAILEZ_LICENSE_PUBKEY. Set it
# explicitly only when service certificates are signed with a separate key.
#
# Output: dist/mailez-ee-<version>/
#   images.tar.gz        all 12 images (8 built + 4 third-party) for docker load
#   ee/docker-compose.ee.yml, install-ee.sh, README-INSTALL-EE.md,
#   VERSION, mailez.env.example
# plus dist/mailez-ee-<version>.tar.gz (+ .sha256) — the shippable bundle.
set -euo pipefail

cd "$(dirname "$0")/.."   # deploy/

VERSION=""
PUBKEY_FILE=""
DEFAULT_PUBKEY_FILE="ee/prod.pub"
while [ $# -gt 0 ]; do
  case "$1" in
    --pubkey-file) PUBKEY_FILE="${2:-}"; shift 2 ;;
    --pubkey-file=*) PUBKEY_FILE="${1#*=}"; shift ;;
    -h|--help|*)
      if [ -z "${VERSION}" ] && [ "$1" != "-h" ] && [ "$1" != "--help" ]; then
        VERSION="$1"; shift
      else
        echo "usage: MAILEZ_LICENSE_PUBKEY=<b64> bash ee/release-ee.sh <version> [--pubkey-file FILE]" >&2
        exit 2
      fi ;;
  esac
done

PUBKEY="${MAILEZ_LICENSE_PUBKEY:-}"
PUBKEY_SOURCE=""
[ -n "${PUBKEY}" ] && PUBKEY_SOURCE="env MAILEZ_LICENSE_PUBKEY"
if [ -n "${PUBKEY_FILE}" ]; then
  [ -f "${PUBKEY_FILE}" ] || { echo "release-ee: pubkey file not found: ${PUBKEY_FILE}" >&2; exit 1; }
  PUBKEY="$(tr -d ' \t\r\n' < "${PUBKEY_FILE}")"
  PUBKEY_SOURCE="file ${PUBKEY_FILE}"
fi
if [ -z "${PUBKEY}" ] && [ -f "${DEFAULT_PUBKEY_FILE}" ]; then
  PUBKEY="$(tr -d ' \t\r\n' < "${DEFAULT_PUBKEY_FILE}")"
  PUBKEY_SOURCE="file ${DEFAULT_PUBKEY_FILE}"
fi
if [ -z "${PUBKEY}" ]; then
  if [ "${MAILEZ_ALLOW_DEVKEY:-0}" = "1" ]; then
    echo "release-ee: WARNING: no MAILEZ_LICENSE_PUBKEY — building a DEV-KEY release." >&2
    echo "            This image can NEVER enforce a license; do not ship it." >&2
  else
    echo "release-ee: refusing to build without MAILEZ_LICENSE_PUBKEY." >&2
    echo "            Looked for the vendor key in ${DEFAULT_PUBKEY_FILE} as well." >&2
    echo "            Generate the production keypair with:" >&2
    echo "              go run -tags mailez_ee ./internal/ee/license/cmd/license genkey" >&2
    echo "            (keep the private key offline; pass the public key here)." >&2
    echo "            For an internal test build set MAILEZ_ALLOW_DEVKEY=1." >&2
    exit 1
  fi
fi
export MAILEZ_LICENSE_PUBKEY="${PUBKEY}"
MAILEZ_SERVICE_PUBKEY="${MAILEZ_SERVICE_PUBKEY:-${PUBKEY}}"
export MAILEZ_SERVICE_PUBKEY
export MAILEZ_IMAGE_TAG="${VERSION}"

COMPOSE_FILES=(-f ee/docker-compose.ee.yml -f ee/docker-compose.build.ee.yml)
ENGINE_IMAGES=(
  "ghcr.io/mailez-hq/mailez-backend-ee:${VERSION}"
  "ghcr.io/mailez-hq/mailez-admin-ee:${VERSION}"
  "ghcr.io/mailez-hq/mailez-webmail-ee:${VERSION}"
  "ghcr.io/mailez-hq/mailez-nginx:${VERSION}"
  "ghcr.io/mailez-hq/mailez-mailezine-ee:${VERSION}"
  "ghcr.io/mailez-hq/mailez-rspamd:${VERSION}"
  "ghcr.io/mailez-hq/mailez-macro-scanner:${VERSION}"
  "ghcr.io/mailez-hq/mailez-unbound:${VERSION}"
)
# Third-party base images the compose file references. redis:alpine,
# minio:latest and rustfs:latest are floating tags: whatever this host
# resolves now is what ships (pin exact tags here once a version policy
# exists).
THIRD_PARTY_IMAGES=(
  "redis:alpine"
  "mysql:8.0"
  "pingcap/tidb:v8.5.1"
  "minio/minio:latest"
  "rustfs/rustfs:latest"
)

# Report where the keys came from, never the key material itself.
if [ -z "${MAILEZ_SERVICE_PUBKEY}" ]; then
  SERVICE_SOURCE="none"
elif [ "${MAILEZ_SERVICE_PUBKEY}" = "${PUBKEY}" ]; then
  SERVICE_SOURCE="same as license"
else
  SERVICE_SOURCE="separate key"
fi
echo "== release-ee ${VERSION} (license pubkey: ${PUBKEY_SOURCE:-DEVKEY}; service pubkey: ${SERVICE_SOURCE}) =="
# deploy/ sits two levels below the workspace root that also holds the
# mailezine checkout (compose resolves the engine context the same way,
# three levels from deploy/ee/).
[ -d ../../mailezine ] || { echo "release-ee: sibling mailezine checkout not found (../../mailezine)" >&2; exit 1; }

echo "== building enterprise images (8) =="
docker compose --env-file mailez.env "${COMPOSE_FILES[@]}" build

echo "== pulling third-party images (4) =="
for img in "${THIRD_PARTY_IMAGES[@]}"; do
  docker pull "$img" || warn "pull failed: $img (will use a local copy if present)"
done
MISSING=0
for img in "${THIRD_PARTY_IMAGES[@]}"; do
  docker image inspect "$img" >/dev/null 2>&1 || { echo "  missing: $img"; MISSING=1; }
done
[ "${MISSING}" = 0 ] || { echo "release-ee: third-party pull failed (offline host? pre-pull them)" >&2; exit 1; }

echo "== verifying built images exist =="
for img in "${ENGINE_IMAGES[@]}"; do
  docker image inspect "$img" >/dev/null 2>&1 || { echo "release-ee: expected image missing: $img" >&2; exit 1; }
done

DIST="dist/mailez-ee-${VERSION}"
rm -rf "${DIST}"
mkdir -p "${DIST}/ee"

echo "== saving image bundle (this takes a while) =="
docker save "${ENGINE_IMAGES[@]}" "${THIRD_PARTY_IMAGES[@]}" \
  | gzip -1 > "${DIST}/images.tar.gz"
( cd "${DIST}" && sha256sum images.tar.gz > images.tar.gz.sha256 )

cp ee/docker-compose.ee.yml "${DIST}/ee/"
cp ee/install-ee.sh "${DIST}/"
cp ee/README-INSTALL-EE.md "${DIST}/"
cp mailez.env.example "${DIST}/"
echo "${VERSION}" > "${DIST}/VERSION"

# Operator overrides ship with the bundle. nginx drop-ins must sit at the top
# level of the installed overrides dir — the gateway config does
# `include /overrides/*.conf;` — while rspamd reads its own subdirectory
# through the mail-filter mount. Without this the installed gateway serves
# webmail only and the admin console stays reachable just on its host port.
mkdir -p "${DIST}/overrides"
if [ -d overrides/nginx ]; then
  cp overrides/nginx/*.conf "${DIST}/overrides/" 2>/dev/null || true
fi
if [ -d overrides/rspamd ]; then
  cp -a overrides/rspamd "${DIST}/overrides/"
fi

echo "== assembling shippable bundle =="
tar -C dist -czf "dist/mailez-ee-${VERSION}.tar.gz" "mailez-ee-${VERSION}"
( cd dist && sha256sum "mailez-ee-${VERSION}.tar.gz" > "mailez-ee-${VERSION}.tar.gz.sha256" )

echo
echo "== done =="
du -sh "${DIST}/images.tar.gz" "dist/mailez-ee-${VERSION}.tar.gz" || true
echo "customer bundle: dist/mailez-ee-${VERSION}.tar.gz (+ .sha256)"
echo "next: verify on a scratch host, then hand to the customer with"
echo "      README-INSTALL-EE.md (install first, license after payment:"
echo "      install-ee.sh activate license.lic service.lic)."
