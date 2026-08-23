#!/bin/sh
# Build the mailez mail-stack images from deploy/vendor/the mail-stack reference and tag them as
# mailez/<component>:local. Run from the repository root.
set -e

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VENDOR="$ROOT/deploy/vendor/mailstack"

docker version >/dev/null 2>&1 || { echo "docker daemon not running" >&2; exit 1; }

build() {
  echo "==> building mailez/$1:local"
  docker build --build-arg VERSION=local -t "mailez/$1:local" "$VENDOR/$2"
}

build base base
for c in nginx dovecot postfix rspamd macro-scanner unbound; do
  build "$c" "$c"
done

echo "All images built. The compose files reference mailez/*:local by default."
