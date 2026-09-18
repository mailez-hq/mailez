# Mailez v1.0.1 Release Notes

- **Release date:** 2026-09-18
- **Release type:** maintenance / patch
- **Previous release:** v1.0.0 (2026-09-14)
- **Upgrade:** set `MAILEZ_IMAGE_TAG=v1.0.1` in `deploy/mailez.env` and run `./deploy/mailezctl.sh up ce`

v1.0.1 is a maintenance release focused on operations automation and bug fixes. The admin console gains four new capabilities — outbound network probes, change-based health alerting, an IP ban engine and scheduled encrypted backups; clients get Delta Chat scan-to-login; bulk flag, move and delete operations become much faster on large mailboxes; and 15 defects are fixed across mail delivery, sorting, deployment and access boundaries.

## 1. New: the admin operations suite

### Outbound network probes

The Health page runs three new **outbound** probes that catch the "everything looks fine here, but the other side never gets the mail" class of failure:

- `outbound25` — reachability of an external port 25 (dials `aspmx.l.google.com:25` by default; point it elsewhere with `MAILEZ_OUTBOUND_PROBE_HOST`)
- `ptr` — whether the mail hostname's PTR record agrees with its EHLO name
- `resolver` — whether the deployment resolves through a public DNS resolver (reputation DNSBLs refuse queries from them, so blacklist screening silently degrades)

### Change-based health alerting

A worker re-runs the health checks on a schedule and alerts **only on status transitions**: the first run records a baseline only, a status has to hold for two consecutive runs before it is announced, and recoveries are reported too.

- hourly by default, tune with `MAILEZ_HEALTH_ALERT_INTERVAL_MIN`
- delivered to the operations mailbox, optionally alongside a JSON webhook (Slack / DingTalk / WeCom compatible)
- `MAILEZ_HEALTH_ALERT_MUTE=system:database,domain:example.com:spf` silences individual checks (key format `system:<id>` or `domain:<name>:<id>`)
- `MAILEZ_HEALTH_ALERT=off` disables the worker entirely

### IP ban engine

Failed authentications are counted per source IP across the web login and the mail-proxy SASL gate: 20 failures within 600 seconds ban the address by default, and the duration escalates on its 30-day history (15 minutes up to 24 hours).

- loopback, the deployment's own addresses and any CIDR in `MAILEZ_BAN_WHITELIST` are exempt, so monitoring probes can never ban themselves
- a new **Bans** page in the admin console lists and lifts bans, with every action audited
- ban activity also feeds the health center

### Scheduled encrypted backups

A daily worker snapshots the control plane (SQLite `VACUUM INTO`) and packages it with the attachment/drive tree, encrypts the result with chunked AES-256-GCM and publishes it to the configured target:

- `MAILEZ_BACKUP_TARGET=local:<dir>` or `s3` (S3-compatible object storage)
- **nothing runs without `MAILEZ_BACKUP_KEY`** — there are no plaintext archives
- 14 archives are kept by default (`MAILEZ_BACKUP_KEEP`), produced at the hour given by `MAILEZ_BACKUP_HOUR`
- the **Backups** page in the admin console shows the configuration and run history, triggers a manual run, and verifies an archive by re-reading and decrypting it from the target
- the health center grades backup freshness: warn past 7 days, fail past 14

## 2. New: Delta Chat scan-to-login

The webmail settings dialog now generates a **DCLOGIN v1 QR code** carrying the email address, a one-time app token and the IMAP/SMTP settings from the deployment's own autoconfig XML. The admin users page adds a per-user "Delta Chat QR" action so operators can hand it out on a user's behalf.

## 3. New: Exchange ActiveSync endpoint (EE)

The gateway had no `/Microsoft-Server-ActiveSync` route, so phones could only reach the loopback backend port and the mail hostname answered 404. `deploy/overrides/nginx/eas.conf` now publishes the endpoint on port 443 (EAS 12.1/14.0/14.1) while the backend port stays loopback-only; phones can set the account up as Exchange directly, and Outlook desktop's autodiscover behaviour is unchanged.

## 4. Performance: bulk operations on large mailboxes

Bulk flag operations no longer cost a transaction per message: the engine commits a batch of flag changes in a single write, and the control plane walks a large folder in windows instead of one pass.

| Operation (300-message mailbox) | v1.0.0 | v1.0.1 |
| --- | --- | --- |
| `UID STORE 1:* +FLAGS \Seen` | ~34 s | **~7 s** |
| `POST /mail/read-all` | ~30 s | **~14 s** |

## 5. Fixes

**Mail delivery and protocols**

- Deleting a message while it was already in Trash did nothing: the API moved Trash → Trash, which the engine treats as a no-op, so the message stayed in the trash forever. `/mail/delete` now flags and UID-EXPUNGEs the message when the folder is Trash (and accepts a `uids` batch); webmail's delete button inside Trash uses it.
- External POP3 aggregation re-delivered every message on each poll: the delivered-UID cursor was written to a column that does not exist, the write failed silently and the cursor never advanced. It is stored in `seen_uid_ls` now, and a failed cursor write is logged instead of dropped.
- Sorting by sender, subject or size returned an empty page: the UIDs in an IMAP `SORT` response were read as integers while go-imap hands them back as strings, so the collector dropped every one.
- A message containing a single very long line (a pasted URL or log line) failed with `mail service error`; bodies are now folded at the RFC 5321 line limit before submission.
- A recipient over quota, or an over-long body line, produced a bare 502; both now answer 422 with `recipient_quota_exceeded` / `line_too_long` and a message the sender can act on.
- `POST /mail/send` ignored the `text` field that `POST /mail/draft` uses for the body, so a caller following the draft API sent an empty message; both field names are accepted.
- Deleting a contact that belongs to another user answered 204; it now answers 404.

**Admin console and webmail**

- Drive share links answered 401 to everyone without a session: the token-only download route sat inside the authenticated group, so the share token never had a chance. It is public now, with optional session so the owner still downloads without a token.
- The sign-in page logged a React hydration error (#418) on every visit: `passkeySupported()` was called during render, so the server emitted the form without the passkey button and the client emitted it with one.
- `/api/v1/events` answered 500 on MySQL-backed deployments whenever the worker token had to be re-minted (`Save` wrote a zero `created_at`, which strict mode rejects), which took out SSE push and the outbox worker for that user.
- The event watcher swallowed the first message that arrived right after a client subscribed; the first check now announces the baseline and runs as soon as the stream connects.
- On deployments that serve the console under `/admin`, the logo, branding and autoconfig XML were still requested from the site root and all three 404'd.
- Webmail's service worker never registered (a TypeScript assertion had slipped into the shipped `sw.js`), which disabled offline support and the notification-click handler.
- Webmail showed the admin console entry to non-admin accounts.
- Web containers ran in UTC rather than the operator's timezone.

**Deploy, operations and dependencies**

- Out-of-order SMTP commands answer 503 instead of go-smtp's 502 (RFC 5321 §4.3.2); the plaintext listeners rewrite that one reply.
- Engine `SORT` reads sort keys from the header block the store caches at delivery instead of opening one blob per message — a 600-message folder used to spend tens of seconds in blob reads.
- The community compose bound the engine's mail ports (25/465/587/110/995/143/993/4190) to 127.0.0.1, so a fresh `mailezctl up ce` deployment could not receive external mail; they now default to `${MAILEZ_MAIL_BIND:-0.0.0.0}` while the debug ports keep the loopback default.
- Deleting a user left the engine-side mailbox behind on the EE stacks: `MAIL_ENGINE_MGMT_ADDR` was passed without a URL scheme and the purge failed with `unsupported protocol scheme "mailezine"`; the compose files now carry `http://`.
- The gateway had no route for `/Microsoft-Server-ActiveSync`, so phones could only use the loopback backend port and the mail hostname answered 404; `deploy/overrides/nginx/eas.conf` publishes it on 443 now (see section 3).
- Operator override compose file, host bind knobs and service-key passthrough for first-deployment hardening.
- `models.AutoMigrate` (the full-schema helper used by tests and tooling) was missing the health-snapshot, ban-record and backup-run tables; production migrations were unaffected.
- Dependency upgrades across the backend Go modules and the frontend workspace (Next.js 16.3.5, React 19.3.0).

**Access control and security**

- Login rate limiting and the ban engine keyed trusted-proxy requests without a forwarded header into one empty-address bucket, so such callers shared a single limit; the socket address is used instead.
- Domain reads are open to the domain's managers: `GET /domains`, `GET /domains/:name`, the DKIM status and the DNS wizard return the domains a manager holds, while every domain mutation (create, update, delete, key generation, managers, alternatives, relays) stays global-admin only.

## 6. Known issues

Everything that was open when this release was prepared is fixed and covered by regression tests, except the two capacity items below. Both were re-checked against the source trees on 2026-09-18; neither is a regression and neither blocks an upgrade.

- **IMAP APPEND ingest runs at roughly 2 messages/s (capacity).** Each APPEND commits through the same delivery path as inbound mail, so bulk import of a large mailbox is slow (10k messages ≈ 1.3 h). Batching that path is a delivery-engine project, not a patch.
- **A stalled full-text index has no health probe (operations).** Body search recovered once the index moved to the v2 format, but an index whose sync has stopped is still invisible to the health center.

Two further notes from the fixes above:

- **`SORT` latency was not re-measured.** The per-message blob read is gone (sort keys come from the cached header block, with a blob fallback for messages that predate the cache), but the end-to-end number on a large folder still has to be re-benchmarked.
- **The SMTP 503 rewrite covers plaintext sessions.** A TLS connection hands this layer ciphertext, which is passed through byte-for-byte, so a client that sends commands out of order inside TLS still sees the library's 502.
- **A stalled full-text index has no health probe (operations).** Body search recovered once the index moved to the v2 format, but an index whose sync has stopped is still invisible to the health center.

## 7. Upgrading

### Standard upgrade

Image deployments: edit `deploy/mailez.env`, point `MAILEZ_IMAGE_TAG` at `v1.0.1`, and bring the stack back up:

```sh
./deploy/mailezctl.sh up ce
```

The control plane runs its versioned migrations on start, and `deploy/data/` is untouched; back that directory up before upgrading.

### New configuration

The following variables can be set in `deploy/mailez.env` (the backend service reads it through `env_file`):

| Variable | Default | Description |
| --- | --- | --- |
| `MAILEZ_OUTBOUND_PROBE_HOST` | empty | Host dialed for the outbound port 25 probe (defaults to `aspmx.l.google.com`) |
| `MAILEZ_HEALTH_ALERT` | `on` | Enables change-based health alerting |
| `MAILEZ_HEALTH_ALERT_INTERVAL_MIN` | `60` | Health check interval, in minutes |
| `MAILEZ_HEALTH_ALERT_WEBHOOK` | empty | Alert webhook URL (Slack / DingTalk / WeCom compatible) |
| `MAILEZ_HEALTH_ALERT_MUTE` | empty | Comma-separated checks to silence, keyed `system:<id>` / `domain:<name>:<id>` |
| `MAILEZ_BAN_MAX_RETRY` | `20` | Failed authentications that trigger a ban |
| `MAILEZ_BAN_FINDTIME_SEC` | `600` | Failure counting window, in seconds |
| `MAILEZ_BAN_WHITELIST` | empty | Comma-separated CIDRs exempt from banning |
| `MAILEZ_BACKUP_KEY` | empty | Backup encryption key; **no backups run without it** |
| `MAILEZ_BACKUP_TARGET` | empty | `local:<dir>` or `s3` |
| `MAILEZ_BACKUP_KEEP` | `14` | Number of archives to keep |
| `MAILEZ_BACKUP_HOUR` | `3` | Hour of the day the backup runs |
| `MAILEZ_MAX_ATTACHMENT_BYTES` | `20971520` (20 MB) | Server-side cap on one attachment; over it the API answers `422 attachment_too_large`. 0 disables the check |

### Behaviour changes to be aware of

- **Community mail-port binding:** the default changes from `127.0.0.1` to `0.0.0.0` (fixing fresh deployments that could not receive external mail). Set `MAILEZ_MAIL_BIND=127.0.0.1` explicitly to keep the old loopback-only behaviour.
- **IP banning is on by default:** 20 failed authentications within 600 seconds ban the source address. Make sure `MAILEZ_BAN_WHITELIST` covers your office egress, monitoring and probe networks before going live.
- **Health alerting is on by default:** hourly checks that announce status transitions (the first run only records a baseline). Set `MAILEZ_HEALTH_ALERT=off` if you don't want it.
- **Backups need a key:** without `MAILEZ_BACKUP_KEY` the backup job does not run and the health center grades backup freshness as failed — set the key and the target before enabling it.
