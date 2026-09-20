# Operations roadmap

Four initiatives that harden day-2 operations for self-hosted deployments.
They share one goal: the administrator should learn about problems before
their users do, and should be able to recover from anything short of total
storage loss. Items are listed in priority order.

| # | Initiative | Theme |
|---|---|---|
| 1 | Change-based health alerting | signal over noise |
| 2 | Scheduled encrypted data backup | recoverability |
| 3 | Outbound network self-checks | deliverability |
| 4 | IP ban engine with admin visibility | abuse resistance |

## 1. Change-based health alerting

**Problem.** The admin digest (daily/weekly operations summary) reports
state on a schedule regardless of whether anything changed. Predictable
email trains administrators to ignore it, and the one red row that matters
arrives buried in twenty green ones.

**Proposal.** Persist each health-check run's result set and notify only on
state *transitions*:

- `ok → warn`, `ok → fail`, `warn → fail`: alert immediately.
- `fail → ok`: send a recovery notice — silence after an alert is how an
  administrator confirms the fix worked.
- Unchanged state: no email. A weekly digest can still exist for
  reassurance.

**Implementation sketch.**

- Health checks already return normalized `ok/warn/fail/unknown` per probe.
  Add a `health_snapshots` table (or KV blob) holding the previous run's
  probe→status map with transition timestamps.
- A background worker (see the workers in `backend/internal/server`) runs
  the check set on a schedule, diffs against the snapshot, and enqueues
  notifications for transitions only.
- Flap suppression: a probe must hold its new state for two consecutive
  runs before the transition is announced — this kills flapping on marginal
  DNS timeouts.
- Per-probe opt-out so chronically noisy probes can be muted.

**Out-of-band notification channels.** Alert delivery must not depend on
the system being monitored. SMTP alerts must target an address *outside*
the deployment's own domains: an expired or suspended domain takes its own
admin mailbox down with it, which is exactly when alerts matter most (the
"renewal reminder sent to the expired domain" circular failure). Ship
email + a generic webhook (Slack/DingTalk/WeCom-compatible JSON body) in
v1.

**Acceptance.** Injecting a DNS failure produces exactly one alert;
restoring it produces exactly one recovery notice; an unchanged system over
a week produces nothing.

## 2. Scheduled encrypted data backup

**Problem.** Config export/import covers control-plane settings only. Mail
data (KV store, message blobs, search indexes) has no scheduled, offsite,
encrypted backup mechanism. For a self-hosted mail platform this is the
trust baseline, not a nice-to-have.

**Proposal.**

- Scheduled incremental, encrypted (AES-256) backups of the full data set
  to pluggable targets: local path, S3-compatible object storage,
  SFTP/rsync.
- Retention policy (e.g. 7 daily + 4 weekly + 6 monthly) with pruning.
- Backup freshness ("days since last successful backup") becomes a health
  probe: `warn` at N days, `fail` at 2N.
- Before/after hooks for operator-specific steps.

**Implementation sketch.**

- The engine's storage layers are pluggable behind the directory contract
  (`/stack/directory/*`). Add snapshot primitives per storage backend —
  consistent copy for SQLite (backup API / `VACUUM INTO`), Pebble
  checkpoint, blob-store sync — rather than file-level copying of live
  stores.
- The backup job runs in the backend worker pool with an explicit state
  machine (`idle → running → last: ok/failed`) exposed via
  `GET /admin/backup/status`.
- Encrypt client-side (key derived from a dedicated secret in
  `mailez.env`) so object-storage targets never hold plaintext.
- A `verify` subcommand restores the catalog and checks integrity without a
  full restore, plus a `cmd/e2e` scenario: seed → backup → wipe → restore →
  assert.

**Acceptance.** A deployment destroyed mid-day recovers to the last
checkpoint with a documented, single-command restore; the health page shows
backup freshness without the operator consulting cron logs.

## 3. Outbound network self-checks

**Problem.** The health center validates the domain-facing half of mail
(MX / SPF / DMARC / DKIM / MTA-STS) but not the network-facing half.
Deployments on cloud providers or residential lines routinely have outbound
port 25 blocked, and a missing or mismatched PTR record silently degrades
delivery. Both surface as the same support ticket: "install succeeded, mail
won't send."

**Proposal.** Three new probes in the health center, each with a localized
fix hint:

- **Outbound port 25 reachability** — dial a well-known MTA (target
  configurable) with a short timeout; on failure, explain that many
  providers block outbound 25 and point at the smarthost relay escape
  hatch.
- **PTR / rDNS consistency** — reverse-resolve the public IP (v4 and v6)
  and compare against the primary hostname used in EHLO; `warn` on
  mismatch, `fail` on missing PTR.
- **Resolver sanity** — when the deployment's resolver is a public DNS
  service, warn that reputation DNSBLs (Spamhaus in particular) refuse
  queries from public resolvers, silently weakening spam screening and
  outbound reputation checks.

**Implementation sketch.** The probes live next to the existing engine
IMAP+MTA dial in the health package. All three are passive: one TCP dial
and two DNS lookups, no mail is sent. IPv6 parity with IPv4 on every probe.

**Acceptance.** A deployment behind a port-25 block shows a red probe with
a remediation hint within one health run of install; the `cmd/e2e`
preflight asserts these checks run on fresh deployments.

## 4. IP ban engine with admin visibility

**Problem.** Login rate limiting exists (`backend/internal/auth`), but
protection against repeated credential stuffing is invisible: operators
cannot see who was blocked, when, or why, and there is no escalation for
repeat offenders. Protection without visibility cannot be audited — and can
silently lock out the operator themselves.

**Proposal.**

- A shared ban engine in mailezine covering four attack surfaces:
  IMAP/POP3 auth, SMTP submission auth (465/587), webmail login, admin
  console login. Counters aggregate per source IP across surfaces.
- Configurable per surface: `maxretry` within `findtime` → `bantime`, with
  exponential escalation for repeat offenders (e.g. the 4th ban in a month
  lasts 24h).
- Self-whitelist: the deployment's own public IPs (v4/v6) are always
  exempt — health probes and monitoring dial the public interface and must
  never ban themselves.
- Admin UI page: banned IPs (IP, surface vector, trigger count, remaining
  time) with manual unban; every ban/unban lands in the audit trail.
- Ban activity feeds the alerting from item 1: "N failed logins from X in
  the last 24h" as a warn-level probe.

**Implementation sketch.** Counters and ban state live in Redis (already in
the stack) with sliding windows; enforcement points hook the SASL path of
the internal stack API (`/stack/*`) and the backend login handlers. The
admin page reads via `GET /admin/bans`; mutations write the audit log.

**Acceptance.** Twenty failed IMAP logins from one IP produce a ban visible
in the admin console within seconds; the operator's own IP survives a
health-check storm; unban is one click and audited.

## Non-goals

- **Hosting authoritative DNS.** mailez stays DNS-provider-agnostic:
  publish the records listed by the DNS wizard, verify them live, and let
  the operator's DNS platform own the zone.
- **Bundling a system metrics stack.** Health probes stay point-in-time;
  host-level resource history belongs to the operator's monitoring.
