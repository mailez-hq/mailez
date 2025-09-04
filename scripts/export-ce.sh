#!/bin/sh
# export-ce: generate the public community source tree from the private
# monorepo (backend + frontend). Run it from the top of the monorepo
# working tree; it always exports the committed local HEAD — never the
# working dir and never a remote (a remote can lag behind and silently
# export stale code):
#
#   scripts/export-ce.sh [public-remote]
#
# Defaults read from the environment:
#   MAILEZ_PUBLIC_URL      public mirror (defaults to the argument)
#   MAILEZ_EXPORT_DIR      scratch clone dir (default: build/export-ce)
#   MAILEZ_EXPORT_CHECK=1  dry-run: run every post-condition, skip the push
#
# The public tree is generated, not mirrored. Every branch is squashed to
# a single root commit built from its filtered HEAD tree, so nothing from
# the private repository's history — old file versions, commit messages,
# pre-refactor wording — can survive in the public object database. The
# public repo holds exactly one commit per branch and no tags; the full
# development history stays private.
#
# Requires git-filter-repo (pip install git-filter-repo).
set -eu

PUBLIC_URL="${1:-${MAILEZ_PUBLIC_URL:-}}"
EXPORT_DIR="${MAILEZ_EXPORT_DIR:-build/export-ce}"
DRY_RUN="${MAILEZ_EXPORT_CHECK:-0}"

command -v git-filter-repo >/dev/null 2>&1 || {
  echo "export-ce: git-filter-repo not found (pip install git-filter-repo)" >&2
  exit 1
}

rm -rf "${EXPORT_DIR}"
# --no-local: a hardlinked local clone makes filter-repo bail out (and would
# rewrite the source objects); force a real object copy.
git clone --mirror --no-local . "${EXPORT_DIR}"
cd "${EXPORT_DIR}"

# A cloned mirror carries private auxiliary refs (stash, notes, replace
# objects, release tags) that must never reach the public remote.
git for-each-ref --format='%(refname)' refs/stash refs/notes refs/replace refs/tags \
  | while read -r ref; do git update-ref -d "${ref}"; done

# The CE/EE boundary contract: the community tree must not know the
# enterprise edition exists. Everything EE lives under backend/internal/ee/,
# in a paired *_ee.go / *_ee_test.go file, under frontend/apps/*/ee/ or
# deploy/ee/, or in one of the private build/process files listed below
# (EE bake targets, ee.mk, the EE Dockerfile, private CI/release workflows,
# the edition-split architecture notes). internal/compose/enterprise*.go and
# the *_ce.go / ce-stubs / set-edition.mjs names are pre-rename locations of
# current files — historical paths obey the same strip rules. The export
# tool itself is private process tooling and must not appear in its own
# output. Internal QA and engine-selection material (e2e-qa/, docs/engines/,
# the webmail UI spec) never belonged to the community tree either.
git filter-repo \
  --invert-paths \
  --path backend/internal/ee/ \
  --path backend/cmd/eas-e2e/ \
  --path backend/cmd/e2e-enterprise/ \
  --path backend/scripts/enterprise-smoke.ps1 \
  --path backend/dev-start.ps1 \
  --path backend/FEATURES.md \
  --path backend/Dockerfile.ee \
  --path frontend/apps/webmail/ee/ \
  --path frontend/apps/admin/ee/ \
  --path frontend/apps/webmail/e2e-qa/ \
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
  --path backend/internal/compose/enterprise_ce.go \
  --path backend/internal/compose/enterprise_ee.go \
  --path backend/internal/compose/enterprise_test.go \
  --path docs/engines/ \
  --path docs/webmail-ui-spec.md \
  --path docs/rollout-runbook.md \
  --path docs/scaling.md \
  --path qa/ \
  --path scripts/export-ce.sh \
  --path .github/workflows/release.yml \
  --path .github/workflows/release-private.yml \
  --path .github/workflows/ci-ee.yml \
  --path docker-bake.ee.hcl \
  --path ee.mk \
  --path-glob '*_ee.go' \
  --path-glob '*_ee_test.go' \
  --path-glob '*_ce.go' \
  --path-glob 'frontend/apps/*/ce-stubs/' \
  --blob-callback '
    # The public tree must not even carry the build-tag marker of the
    # private variant: drop the tag line from otherwise-public files.
    data = blob.data
    if data.startswith(b"//go:build !mailez_ee\n\n"):
        data = data[len(b"//go:build !mailez_ee\n\n"):]
    elif data.startswith(b"//go:build !mailez_ee\n"):
        data = data[len(b"//go:build !mailez_ee\n"):]
    blob.data = data
  '

# Squash every branch to a single root commit built from its filtered
# tree. This is what makes the export content-complete: a plain history
# filter keeps the old commits, and everything their blobs mention; after
# squashing, no object from the private history remains in the public
# repo at all.
git for-each-ref --format='%(refname)' refs/heads | while read -r ref; do
  tree="$(git rev-parse "${ref}^{tree}")"
  date="$(git log -1 --format=%aI "${ref}")"
  commit="$(
    GIT_AUTHOR_NAME=mailez-export \
    GIT_AUTHOR_EMAIL=export@mailez.invalid \
    GIT_AUTHOR_DATE="${date}" \
    GIT_COMMITTER_NAME=mailez-export \
    GIT_COMMITTER_EMAIL=export@mailez.invalid \
    GIT_COMMITTER_DATE="${date}" \
    git commit-tree "${tree}" -m "mailez public source export"
  )"
  git update-ref "${ref}" "${commit}"
done

# Post-conditions over the final public refs (one commit each): private
# paths must not survive under any name (current or pre-rename), and no
# file content may mention the private variant or its build machinery.
if git log --all --name-only --format= | sort -u | grep -E 'backend/internal/ee/|_ee\.go$|_ee_test\.go$|_ce\.go$|(^|/)ee\.mk$|(^|/)Dockerfile\.ee$|^docker-bake\.ee\.hcl$|^\.github/workflows/(ci-ee|release-private|release)\.yml$|^scripts/export-ce\.sh$|^backend/cmd/(eas-e2e|e2e-enterprise)/|enterprise-smoke|^backend/dev-start\.ps1$|^backend/FEATURES\.md$|^docs/(rollout-runbook|scaling|webmail-ui-spec)\.md$|^docs/engines/|^frontend/apps/[^/]*/(ee|ce-stubs)/|^frontend/apps/webmail/e2e-qa/|^deploy/ee/|^deploy/docker-compose\.(build\.)?(ee|enterprise|ha|multi)|^backend/internal/compose/enterprise|(^|/)set-edition\.mjs$'; then
  echo "export-ce: enterprise paths survived the filter (history)" >&2
  exit 1
fi

forbidden='enterprise|mailez_ee|internal/ee|check-ce-purity|export-ce|docker-bake\.ee|ci-ee|Dockerfile\.ee|ee\.mk|FEATURES\.md|MAILEZ_EDITION|MAILEZ_LICENSE|NEXT_PUBLIC_MAILEZ_EDITION|NEXT_PUBLIC_MAILEZ_FULL|ce-stubs|@/edition|set-edition'
for ref in $(git for-each-ref --format='%(refname)' refs/heads); do
  if git grep -I -i -E "${forbidden}" "${ref}" | grep .; then
    echo "export-ce: forbidden content survived in ${ref}" >&2
    exit 1
  fi
done

# The filtered tree must build and test on its own; run those gates in the
# public mirror's CI as a release gate as well.

if [ "${DRY_RUN}" = "1" ]; then
  echo "export-ce: check mode (MAILEZ_EXPORT_CHECK=1); all post-conditions passed, not pushed"
elif [ -z "${PUBLIC_URL}" ]; then
  echo "export-ce: pass a public remote to push (or MAILEZ_EXPORT_CHECK=1 for a dry run)" >&2
  exit 1
else
  # Restricted refspec (never --mirror): branches and tags only, so any
  # auxiliary ref that reappears locally cannot be pushed by accident.
  git push --force "${PUBLIC_URL}" 'refs/heads/*:refs/heads/*' 'refs/tags/*:refs/tags/*'
  echo "export-ce: pushed ${PUBLIC_URL}"
fi
