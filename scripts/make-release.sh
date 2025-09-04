#!/usr/bin/env bash
# make-release.sh — pack the deploy-only release tarball.
#
# The tarball is image-only: no sources, no build files. Contents
# (top-level dir mailez-<version>/):
#   install.sh                      interactive installer (from scripts/)
#   VERSION                         release version
#   deploy/docker-compose.ce.yml
#   deploy/mailezctl.sh  deploy/mailez.env.example
#   docs/{architecture,upgrades}.md
#
# Source resolution (first match wins):
#   1. $MAILEZ_RELEASE_SOURCE      explicit tree to pack from
#   2. build/ce-worktree           CE export working tree (public tarball
#                                  is packed from CE-published content)
#   3. this checkout               fallback (release CI: the tagged tree)
#
# Usage:
#   VERSION=v1.0.0 ./scripts/make-release.sh      # or: ./scripts/make-release.sh v1.0.0
# Output:
#   dist/mailez-<version>.tar.gz
#   dist/checksums.txt                             sha256 of the tarball
set -euo pipefail

VERSION="${1:-${VERSION:-}}"
if [ -z "$VERSION" ]; then
  VERSION="$(git describe --tags --always 2>/dev/null || echo dev)"
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

SRC="${MAILEZ_RELEASE_SOURCE:-}"
if [ -z "$SRC" ] && [ -d "$ROOT/build/ce-worktree/deploy" ]; then
  SRC="$ROOT/build/ce-worktree"
fi
[ -z "$SRC" ] && SRC="$ROOT"

# Each file resolves from the source tree, falling back to this checkout
# (e.g. a fresh installer not yet re-exported into the CE worktree).
stage() { # stage <relpath> [dest]
  local rel="$1" dest="${2:-$1}"
  local from="$SRC/$rel"
  [ -f "$from" ] || from="$ROOT/$rel"
  [ -f "$from" ] || { echo "missing: $rel" >&2; exit 1; }
  mkdir -p "$STAGE/$(dirname "$dest")"
  # normalize CRLF -> LF (sources may come from a Windows checkout) and
  # keep the exec bit for scripts (redirection drops the mode bits)
  sed 's/\r$//' "$from" > "$STAGE/$dest"
  case "$dest" in *.sh) chmod 755 "$STAGE/$dest" ;; esac
}

STAGE_ROOT="$(mktemp -d)"
trap 'rm -rf "$STAGE_ROOT"' EXIT
TOP="mailez-$VERSION"
STAGE="$STAGE_ROOT/$TOP"
mkdir -p "$STAGE"

# Tarball contents: the deployment recipes and docs a fresh production
# install needs.
stage deploy/docker-compose.ce.yml
stage deploy/mailezctl.sh
stage deploy/mailez.env.example
stage docs/architecture.md
stage docs/upgrades.md
stage scripts/install.sh install.sh
printf '%s\n' "$VERSION" > "$STAGE/VERSION"

mkdir -p "$ROOT/dist"
TARBALL="mailez-$VERSION.tar.gz"
tar -czf "$ROOT/dist/$TARBALL" -C "$STAGE_ROOT" "$TOP"
( cd "$ROOT/dist" && sha256sum "$TARBALL" > checksums.txt )

echo "release tarball: dist/$TARBALL (from: $SRC)"
