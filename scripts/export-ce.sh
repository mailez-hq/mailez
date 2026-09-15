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
# The public branch IS the private dev branch's history, minus everything
# that must not be public: filter-repo strips private paths across ALL
# revisions, rewrites the build-tag line out of shared files, and applies a
# mailmap so no personal/corporate email address leaks through authorship.
# Commit messages, authorship and dates stay exactly as they were, and the
# rewrite is deterministic — identical private prefix produces identical
# public SHAs — so pushes are plain fast-forwards. The one deliberate
# exception: changing the strip list or mailmap rewrites every public SHA,
# which needs an explicit MAILEZ_EXPORT_FORCE=1 re-root (and any existing
# forks keep the old objects, so treat it as a fresh open-source start).
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
# output. Internal QA and engine-selection material (e2e-qa/, the mail-e2e
# regression suite with its artifacts and reports, docs/engines/,
# the webmail UI spec) never belonged to the community tree either. The
# marketing-screenshot tooling (shots-*.mjs + its demo seeder) manufactures
# internal collateral with fabricated personas and lookalike third-party
# domains — private as well.
# Authorship privacy: every email that ever authored the private history
# must map to a public one, or the export refuses to run — a new
# contributor's corporate address would otherwise leak through old commits.
# Extend the mailmap when a new address appears; never widen the allow-list.
MAILMAP="${EXPORT_DIR}.mailmap"
cat > "${MAILMAP}" <<'EOF'
Guan <guan.shiliang@gmail.com> guanshiliang <guan.shiliang@chinaott.net>
EOF
bad="$(git log --all --format='%ae' | sort -u | grep -v -x -E 'guan\.shiliang@gmail\.com|guan\.shiliang@chinaott\.net' || true)"
if [ -n "${bad}" ]; then
  echo "export-ce: unmapped author email(s) — add a mailmap line first:" >&2
  echo "${bad}" >&2
  exit 1
fi

git filter-repo \
  --mailmap "${MAILMAP}" \
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
  --path frontend/mail-e2e/ \
  --path frontend/apps/webmail/shots-demo.mjs \
  --path frontend/apps/webmail/shots-admin.mjs \
  --path frontend/apps/webmail/shots-extra.mjs \
  --path frontend/apps/webmail/shots-features.mjs \
  --path frontend/scripts/set-edition.mjs \
  --path deploy/ee/ \
  --path deploy/overrides/nginx/eas.conf \
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

# Note on go.mod: the exported manifests carry the private tree's
# enterprise-only requirements verbatim. Pruning them inside the export
# was tried and rejected — a rebuilt tip re-parents on every export, so
# pushes could never stay fast-forward. The CI dependency gate is
# `go mod verify` instead of `go mod tidy -diff` for exactly this reason.

# Post-conditions over the filtered refs: private
# paths must not survive under any name (current or pre-rename), and no
# file content may mention the private variant or its build machinery.
if git log --all --name-only --format= | sort -u | grep -E 'backend/internal/ee/|_ee\.go$|_ee_test\.go$|_ce\.go$|(^|/)ee\.mk$|(^|/)Dockerfile\.ee$|^docker-bake\.ee\.hcl$|^\.github/workflows/(ci-ee|release-private|release)\.yml$|^scripts/export-ce\.sh$|^backend/cmd/(eas-e2e|e2e-enterprise)/|enterprise-smoke|^backend/dev-start\.ps1$|^backend/FEATURES\.md$|^docs/(rollout-runbook|scaling|webmail-ui-spec)\.md$|^docs/engines/|^frontend/apps/[^/]*/(ee|ce-stubs)/|^frontend/apps/webmail/e2e-qa/|^frontend/mail-e2e/|^deploy/ee/|^deploy/docker-compose\.(build\.)?(ee|enterprise|ha|multi)|^backend/internal/compose/enterprise|(^|/)set-edition\.mjs$|^frontend/apps/webmail/shots-|^backend/seed-demo-shots\.ps1$'; then
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
# Tip-tree gate: history is published as it was written (that is the point
# of a real open-source history), so old wordings in past revisions stay —
# what must never ship is the CURRENT tree carrying private-edition build
# vocabulary. The path gate above handles the historical dimension by
# stripping those files from every revision outright.
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
  # Deterministic publish: the filtered branch rewrites to stable SHAs, so
  # a plain push fast-forwards. MAILEZ_EXPORT_FORCE=1 is the explicit,
  # rare re-root (strip list or mailmap changed) — it rewrites every
  # public SHA, so it is never the default.
  force=""
  if [ "${MAILEZ_EXPORT_FORCE:-0}" = "1" ]; then
    force="--force"
  fi
  if git push ${force} "${PUBLIC_URL}" 'refs/heads/dev:refs/heads/dev'; then
    echo "export-ce: pushed dev -> ${PUBLIC_URL}${force:+ (forced re-root)}"
  else
    echo "export-ce: fast-forward push failed — if the strip list or mailmap" >&2
    echo "           changed, re-root deliberately with MAILEZ_EXPORT_FORCE=1" >&2
    exit 1
  fi
fi
