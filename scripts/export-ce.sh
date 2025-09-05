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
# The public tree is generated, not mirrored. The public branch took root at
# a hand-made "Initial commit" snapshot (built from a filtered tree, so no
# private object ever entered the public repo) and evolves from there: each
# export applies the freshly filtered HEAD tree as ONE fast-forward "sync
# from mailez@<src-sha>" commit. Nothing from the private repository's
# history — old file versions, commit messages, pre-refactor wording, the
# filtered mirror's own intermediate revisions — is ever published; the
# public branch is fast-forward only and never force-pushed.
#
# Requires git-filter-repo (pip install git-filter-repo).
set -eu

PUBLIC_URL="${1:-${MAILEZ_PUBLIC_URL:-}}"
EXPORT_DIR="${MAILEZ_EXPORT_DIR:-build/export-ce}"
DRY_RUN="${MAILEZ_EXPORT_CHECK:-0}"
# The push phase reads this dir from inside the public clone; make it
# absolute up front so a relative default survives those cd's.
EXPORT_DIR="$(pwd)/${EXPORT_DIR}"

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
# the webmail UI spec) never belonged to the community tree either. The
# marketing-screenshot tooling (shots-*.mjs + its demo seeder) manufactures
# internal collateral with fabricated personas and lookalike third-party
# domains — private as well.
git filter-repo \
  --invert-paths \
  --path backend/internal/ee/ \
  --path backend/cmd/eas-e2e/ \
  --path backend/cmd/e2e-enterprise/ \
  --path backend/scripts/enterprise-smoke.ps1 \
  --path backend/dev-start.ps1 \
  --path backend/FEATURES.md \
  --path backend/Dockerfile.ee \
  --path backend/seed-demo-shots.ps1 \
  --path frontend/apps/webmail/ee/ \
  --path frontend/apps/admin/ee/ \
  --path frontend/apps/webmail/e2e-qa/ \
  --path frontend/apps/webmail/shots-demo.mjs \
  --path frontend/apps/webmail/shots-admin.mjs \
  --path frontend/apps/webmail/shots-extra.mjs \
  --path frontend/apps/webmail/shots-features.mjs \
  --path frontend/scripts/set-edition.mjs \
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

# The public repo's history starts at a hand-made "Initial commit" root and
# then evolves normally: this script does NOT publish or rewrite public
# history. It only produces the filtered tree here (the mirror keeps its
# filtered history — which makes the path post-condition below scan every
# filtered revision, not just the tip) and, in the push phase, applies that
# tree as a single "sync from mailez@<src-sha>" commit on top of the public
# branch. Fast-forward only, never a force.

# Post-conditions over the filtered refs: private
# paths must not survive under any name (current or pre-rename), and no
# file content may mention the private variant or its build machinery.
if git log --all --name-only --format= | sort -u | grep -E 'backend/internal/ee/|_ee\.go$|_ee_test\.go$|_ce\.go$|(^|/)ee\.mk$|(^|/)Dockerfile\.ee$|^docker-bake\.ee\.hcl$|^\.github/workflows/(ci-ee|release-private|release)\.yml$|^scripts/export-ce\.sh$|^backend/cmd/(eas-e2e|e2e-enterprise)/|enterprise-smoke|^backend/dev-start\.ps1$|^backend/FEATURES\.md$|^docs/(rollout-runbook|scaling|webmail-ui-spec)\.md$|^docs/engines/|^frontend/apps/[^/]*/(ee|ce-stubs)/|^frontend/apps/webmail/e2e-qa/|^deploy/ee/|^deploy/docker-compose\.(build\.)?(ee|enterprise|ha|multi)|^backend/internal/compose/enterprise|(^|/)set-edition\.mjs$|^frontend/apps/webmail/shots-|^backend/seed-demo-shots\.ps1$'; then
  echo "export-ce: enterprise paths survived the filter (history)" >&2
  exit 1
fi

# Content gate: the private docs and process tooling were written in
# Chinese, so an ASCII-only word list misses whole EE vocabulary. The
# list below covers both the English build-machinery terms and the CJK
# product words; any surviving occurrence — in history or in files —
# fails the export. Keep it intentionally strict: if a public file ever
# needs to mention the paid edition, the wording must not use these
# tokens (and the private wording lives in the stripped docs instead).
forbidden='enterprise|mailez_ee|internal/ee|check-ce-purity|export-ce|docker-bake\.ee|ci-ee|Dockerfile\.ee|ee\.mk|FEATURES\.md|MAILEZ_EDITION|MAILEZ_LICENSE|NEXT_PUBLIC_MAILEZ_EDITION|NEXT_PUBLIC_MAILEZ_FULL|ce-stubs|@/edition|set-edition|多活|企业版|商业版'
# Tip-tree gate: the public branch's history is exactly the sequence of
# these gated snapshots (one "sync" commit per export), and the filtered
# mirror's internal history is never published — so the tip gate covers
# everything the public repo can ever contain. Old wordings inside the
# private mirror's history stay private by construction.
if git grep -I -i -E "${forbidden}" refs/heads/dev | grep .; then
  echo "export-ce: forbidden content survived in dev" >&2
  exit 1
fi

# The filtered tree must build and test on its own; run those gates in the
# public mirror's CI as a release gate as well.

if [ "${DRY_RUN}" = "1" ]; then
  echo "export-ce: check mode (MAILEZ_EXPORT_CHECK=1); all post-conditions passed, not pushed"
elif [ -z "${PUBLIC_URL}" ]; then
  echo "export-ce: pass a public remote to push (or MAILEZ_EXPORT_CHECK=1 for a dry run)" >&2
  exit 1
else
  # Incremental publish: on top of the public branch (rooted at the initial
  # "Initial commit" snapshot), apply the filtered tree as ONE sync commit.
  # The public branch is fast-forward only — this script never rewrites,
  # force-pushes, or re-roots public history. If the public branch is ever
  # diverged, the push fails loudly instead of being forced.
  PUB_DIR="${EXPORT_DIR}-pub"
  rm -rf "${PUB_DIR}"
  git clone --no-local --quiet "${PUBLIC_URL}" "${PUB_DIR}"
  cd "${PUB_DIR}"
  if ! git rev-parse --verify -q refs/heads/dev >/dev/null; then
    echo "export-ce: public remote has no dev branch; initialize it with a" >&2
    echo "           single \"Initial commit\" root before the first export." >&2
    exit 1
  fi
  cur_tree="$(git rev-parse refs/heads/dev^{tree})"
  new_tree="$(git --git-dir="${EXPORT_DIR}" rev-parse "refs/heads/dev^{tree}")"
  if [ "${new_tree}" = "${cur_tree}" ]; then
    echo "export-ce: public dev already matches the filtered tree (in sync)"
  else
    src_sha="$(git --git-dir="${EXPORT_DIR}" log -1 --format=%H refs/heads/dev)"
    src_date="$(git --git-dir="${EXPORT_DIR}" log -1 --format=%cI refs/heads/dev)"
    git fetch --quiet "${EXPORT_DIR}" "refs/heads/dev:refs/temp/filtered"
    new_tree="$(git rev-parse refs/temp/filtered^{tree})"
    commit="$(
      GIT_AUTHOR_NAME=mailez-export \
      GIT_AUTHOR_EMAIL=export@mailez.net \
      GIT_AUTHOR_DATE="${src_date}" \
      GIT_COMMITTER_NAME=mailez-export \
      GIT_COMMITTER_EMAIL=export@mailez.net \
      GIT_COMMITTER_DATE="${src_date}" \
      git commit-tree "${new_tree}" -p refs/heads/dev -m "sync from mailez@${src_sha}"
    )"
    git update-ref refs/heads/dev "${commit}"
    git push --quiet "${PUBLIC_URL}" "refs/heads/dev:refs/heads/dev" || {
      echo "export-ce: public push failed (diverged history?) — not forcing" >&2
      exit 1
    }
    echo "export-ce: synced dev $(git rev-parse --short refs/heads/dev) (from mailez@${src_sha})"
  fi
  cd .. && rm -rf "${PUB_DIR}"
fi
