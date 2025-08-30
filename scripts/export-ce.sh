#!/bin/sh
# export-ce: generate the public community source tree from the private
# monorepo (backend + frontend). Usage:
#
#   scripts/export-ce.sh [public-remote]
#
# Strips every enterprise-only path and paired *_ee.go file, keeps the full
# commit history (filter-repo is deterministic: unchanged history rewrites
# to identical SHAs, so pushes to the public mirror stay fast-forward while
# main is append-only). Requires git-filter-repo.
set -eu

PUBLIC_URL="${1:-${MAILEZ_PUBLIC_URL:-}}"
EXPORT_DIR="${MAILEZ_EXPORT_DIR:-build/export-ce}"

command -v git-filter-repo >/dev/null 2>&1 || {
  echo "export-ce: git-filter-repo not found (pip install git-filter-repo)" >&2
  exit 1
}

rm -rf "${EXPORT_DIR}"
# --no-local: a hardlinked local clone makes filter-repo bail out (and would
# rewrite the source objects); force a real object copy.
git clone --mirror --no-local . "${EXPORT_DIR}"
cd "${EXPORT_DIR}"

# A cloned mirror carries private auxiliary refs (refs/stash etc.) that
# filter-repo does not rewrite — they must never reach the public remote.
git for-each-ref --format='%(refname)' refs/stash refs/notes refs/replace \
  | while read -r ref; do git update-ref -d "${ref}"; done

# The CE/EE boundary contract (see backend/FEATURES.md): everything
# enterprise lives under backend/internal/ee, frontend ee/ directories, or
# a paired *_ee.go file. The export tool itself is private monorepo
# process tooling and must not appear in its own output.
git filter-repo \
  --invert-paths \
  --path backend/internal/ee/ \
  --path backend/cmd/eas-e2e/ \
  --path frontend/apps/webmail/ee/ \
  --path frontend/apps/admin/ee/ \
  --path scripts/export-ce.sh \
  --path-glob '*_ee.go' \
  --path-glob '*_ee_test.go'

if git ls-tree -r --name-only HEAD | grep -E 'backend/internal/ee/|frontend/apps/[^/]*/ee/|_ee\.go$|_ee_test\.go$|^scripts/export-ce\.sh$'; then
  echo "export-ce: enterprise paths survived the filter" >&2
  exit 1
fi

# Post-condition: the public tree must build and test on its own; run this
# in the public mirror's CI as a release gate as well.

if [ -n "${PUBLIC_URL}" ]; then
  # Restricted refspec (never --mirror): branches and tags only, so any
  # auxiliary ref that reappears locally cannot be pushed by accident.
  git push --force "${PUBLIC_URL}" 'refs/heads/*:refs/heads/*' 'refs/tags/*:refs/tags/*'
  echo "export-ce: pushed ${PUBLIC_URL}"
else
  echo "export-ce: filtered mirror ready at ${EXPORT_DIR} (pass a remote to push)"
fi
