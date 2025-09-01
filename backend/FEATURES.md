# Edition split: community (CE) / enterprise (EE)

The mailez backend ships in two editions from one repository. Both editions
run the same mailezine engine and the same protocols; the community edition
is the open-source webmail/console core on single-node storage, while the
enterprise edition adds distributed storage readiness, compliance,
collaboration-at-scale, identity integration and AI. The split is enforced
by the compiler: EE code lives only under `backend/internal/ee/` (plus
paired `*_ee.go` assembly files), guarded by the `mailez_ee` build tag.

| Capability | CE | EE |
|---|---|---|
| Webmail, compose/outbox, contacts, sieve rules | ✓ | ✓ |
| Personal calendar + ICS feeds, personal drive | ✓ | ✓ |
| Calendar sharing | ✓ (1 read-only share per user) | ✓ (unlimited, read/write) |
| Drive share links | ✓ (3 active links per user) | ✓ (unlimited) |
| Admin console (domains, users, config) | ✓ | ✓ |
| Branding (login/console customization) | | ✓ `*_ee.go` seam |
| Audit log (admin view) | ✓ (raw method/path) | ✓ (+ semantic action/target/detail, CSV export) |
| OIDC federated login (SSO) | | ✓ `internal/auth` (EE-tagged) |
| External account aggregation (fetch), push/SSE | ✓ | ✓ |
| CardDAV/CalDAV personal sync, invite links | ✓ | ✓ |
| AD/LDAP directory sync (login + address book) | | ✓ `internal/ee/ldap` |
| Delegation (授权代发/代管) | | ✓ `internal/ee/delegation` |
| Announcements (公告) | | ✓ `internal/ee/announcement` |
| Compliance archive (audit + retention) | | ✓ `internal/ee/archive` |
| DLP (敏感词/审批 outbound filter) | | ✓ `internal/ee/dlp` |
| AI assistant (providers + chat) | | ✓ `internal/ee/ai` |
| ActiveSync | | ✓ `internal/ee/activesync` |
| S/MIME (certs, sign/encrypt) | | ✓ `internal/ee/smime` |
| Macro scanner (attachment VBA analysis) | ✓ `internal/macro` | ✓ |
| Mail-data backup (engine snapshots, config export/import) | ✓ | ✓ |

Backend frontends (next.js): enterprise UI modules live in
`frontend/apps/<app>/ee/` and are materialized into `edition/` (a
gitignored build artifact) from `ee/` (default) or `ce-stubs/`
(MAILEZ_EDITION=ce) by `frontend/scripts/set-edition.mjs`, which both
`next.config.ts` files run automatically. Code imports them only through
`@/edition/<module>`. Migrated so far: webmail delegations/S-MIME/AI
summary; admin announcement/archive/DLP pages, enterprise config tabs
(AI/LDAP) and the audit-export feature flag (thin route shells re-export
the edition module). Sharing (calendar/drive) is community-side: the webmail
UI ships in CE with quota copy and revoke buttons, and the per-edition
limits live in `*_ce.go`/`*_ee.go` quota files.

\* Calendar/drive sharing is quota-gated in CE (1 read-only calendar share,
3 active drive links) and unlimited in EE; the implementation lives in
shared packages with per-edition quota policy files.

Rules (same contract as the mailezine engine):

- The CE dependency graph must never reach `backend/internal/ee`
  (`make check-ce-purity`); the public frontend tree never contains
  `frontend/apps/*/ee/`.
- EE imports CE freely; CE reaches EE only through seams: the server-level
  `eeRegisterRoutes`/`eeStartComplianceWorkers`/`eeNewDirectorySync`/
  `eeWireAppDirectory` paired files, the compose `OutboundScanner`
  interface, `core.DirectorySync`, `stack.GroupResolver`, per-handler
  `*_ce.go` stubs (admin AI/LDAP routes, user S/MIME routes, calendar
  sharing, drive share links), and the frontend `@/edition/*` modules.
- Degrade, never wedge: CE binaries warn "requires the enterprise edition"
  where configuration implies an EE capability; `cmd/eas-e2e` prints a
  clear error in CE builds; frontend CE stubs show an enterprise
  placeholder instead of the feature.
- The public CE tree is generated from the private monorepo by an export
  tool that lives outside the CE tree; it strips `backend/internal/ee/`,
  `backend/cmd/eas-e2e/` (the EE-only EAS test driver), `frontend/apps/*/ee/`,
  `*_ee.go`, `*_ee_test.go`, ships `README.ce.md`
  as `README.md` instead of the private README, and must build and test
  standalone.
- `cmd/mailez-agent` and `internal/{agent,macro}` are shared
  infrastructure (the mail images' container entrypoint and the macro
  scanner used by every compose profile), so they live in the community
  tree; only the legacy pre-mailezine engine drivers were removed with the
  old stack.

Not yet split (follow-up, mixed concerns or in-flight user work):

- AiSection in webmail profile settings (a personal AI preference toggle)
  stays CE; the AI feature itself is EE.
