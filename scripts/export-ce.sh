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

# The community README ships instead of the internal one; capture its
# expected content from the source HEAD for the post-condition below.
CE_README="$(git show HEAD:README.ce.md)"

# A cloned mirror carries private auxiliary refs (refs/stash etc.) that
# filter-repo does not rewrite — they must never reach the public remote.
git for-each-ref --format='%(refname)' refs/stash refs/notes refs/replace \
  | while read -r ref; do git update-ref -d "${ref}"; done

# The CE/EE boundary contract (see backend/FEATURES.md): everything
# enterprise lives under backend/internal/ee, frontend ee/ directories, or
# a paired *_ee.go file; enterprise deployment recipes live under
# deploy/ee/; private CI (ci-ee.yml) and the EE bake targets
# (docker-bake.ee.hcl) stay private as well; internal runbooks
# (docs/rollout-runbook.md) and the enterprise scaling guide
# (docs/scaling.md) are private operations material. The export tool
# itself is private monorepo process tooling and must not appear in its
# own output; the release workflow stays private too (the private repo is
# the single image publisher — a public run would push CE-built *-ee
# images over the real ones on shared tags); the private README.md is
# replaced by README.ce.md (second pass below).
#
# History is rewritten, so renames matter as much as current paths: every
# pre-rename location of a stripped file (deploy/docker-compose.*.yml
# recipes, the compose/enterprise.go pair) is listed explicitly, otherwise
# the old blobs survive under their historical paths. Internal QA and
# engine-selection material (e2e-qa/, docs/engines/, the webmail UI spec)
# never belonged to the community tree either.
git filter-repo \
  --invert-paths \
  --path backend/internal/ee/ \
  --path backend/cmd/eas-e2e/ \
  --path backend/cmd/e2e-enterprise/ \
  --path backend/scripts/enterprise-smoke.ps1 \
  --path frontend/apps/webmail/ee/ \
  --path frontend/apps/admin/ee/ \
  --path deploy/ee/ \
  --path deploy/docker-compose.ee.yml \
  --path deploy/docker-compose.ee.dev.yml \
  --path deploy/docker-compose.build.ee.yml \
  --path deploy/docker-compose.ha.yml \
  --path deploy/docker-compose.multi.yml \
  --path deploy/docker-compose.build.multi.yml \
  --path deploy/docker-compose.enterprise.yml \
  --path deploy/docker-compose.enterprise.dev.yml \
  --path deploy/docker-compose.build.enterprise.yml \
  --path backend/internal/compose/enterprise.go \
  --path backend/internal/compose/enterprise_test.go \
  --path frontend/apps/webmail/e2e-qa/ \
  --path docs/engines/ \
  --path docs/webmail-ui-spec.md \
  --path scripts/export-ce.sh \
  --path .github/workflows/release.yml \
  --path .github/workflows/ci-ee.yml \
  --path docker-bake.ee.hcl \
  --path docs/rollout-runbook.md \
  --path docs/scaling.md \
  --path README.md \
  --path-glob '*_ee.go' \
  --path-glob '*_ee_test.go'

# Second pass: publish the community README under the canonical name.
git filter-repo --force --path-rename README.ce.md:README.md

# Post-condition over the FULL rewritten history (not just HEAD): renamed
# files must not survive under any of their historical paths either.
if git log --all --name-only --format= | sort -u | grep -E 'backend/internal/ee/|frontend/apps/[^/]*/ee/|_ee\.go$|_ee_test\.go$|^deploy/ee/|^backend/cmd/(eas-e2e|e2e-enterprise)/|enterprise-smoke|^\.github/workflows/(ci-ee|release)\.yml$|^docker-bake\.ee\.hcl$|^docs/(rollout-runbook|scaling|webmail-ui-spec)\.md$|^docs/engines/|^scripts/export-ce\.sh$|^frontend/apps/webmail/e2e-qa/|^deploy/docker-compose\.(build\.)?(ee|enterprise|ha|multi)|^backend/internal/compose/enterprise(_test)?\.go$'; then
  echo "export-ce: enterprise paths survived the filter (history)" >&2
  exit 1
fi
if [ "$(git show HEAD:README.md)" != "$CE_README" ]; then
  echo "export-ce: README.md is not the community README (README.ce.md)" >&2
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
