# Changelog

All notable changes to mailez are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and adheres to
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed

- `install.sh` reported success when seeding the admin account failed: the
  failure was downgraded to a `seed failed (maybe already seeded)` warning,
  which left a running stack with no admin account at all. A failed seed is
  now a failed install, with the command to re-run once the cause is fixed
- Passkey registration failed before a credential was ever created, with
  `Failed to execute 'atob' on 'Window': The string to be decoded contains
  characters outside of the Latin1 range`. The ceremony was configured with
  `EncodeUserIDAsString`, which puts the user handle into a JSON string,
  while `WebAuthnID()` returns the raw 32 bytes of `sha256(email)`:
  `encoding/json` replaced every byte that is not valid UTF-8 with `U+FFFD`
  and the webmail's `atob()` rejected the result. The handle now leaves as
  the base64url string the specification asks for (go-webauthn's default),
  which is what the webmail decodes
- `install.sh` dropped the image tag: it rewrote `MAILEZ_IMAGE_TAG` in place,
  but the shipped `mailez.env.example` keeps that key commented out, so the
  substitution matched nothing, the generated `deploy/mailez.env` carried no
  tag at all and the stack fell back to pulling `:latest`. `--tag` and the
  interactive prompt now land in the file (the closing summary also prints the
  admin address as `admin@<MAILEZ_DOMAIN>`)
- `mailez-seed` panicked with `slice bounds out of range [:-1]` on a
  deployment whose `MAILEZ_DOMAIN` was as long as the default
  `admin@example.com`: the local part was cut off the email by subtracting
  the domain length, so equal lengths meant a negative bound (and any other
  mismatch wrote a wrong local part in silence). The account is now
  `admin@<MAILEZ_DOMAIN>` by default, and an explicit `MAILEZ_ADMIN_EMAIL`
  outside that domain stops with a message naming both values

## [1.0.1] - 2026-09-18

### Added

- Server-side per-attachment size limit (`MAILEZ_MAX_ATTACHMENT_BYTES`,
  20 MiB by default, matching the webmail picker): the API now refuses a
  file the picker itself would reject, answering `422 attachment_too_large`.
  Set it to 0 to disable the check
- Delta Chat onboarding: a DCLOGIN v1 login QR (email address + one-time
  app token + IMAP/SMTP settings from the deployment's own autoconfig
  XML) generated in the webmail settings dialog, plus a per-user
  "Delta Chat QR" action in the admin users page
- Admin sidebar: quick Webmail entry and a theme-coloured console lockup
- Outbound network probes on the health page: outbound port 25
  reachability (probe host overridable via `MAILEZ_OUTBOUND_PROBE_HOST`),
  PTR/EHLO hostname agreement for the mail hostname, and public-resolver
  detection (reputation DNSBLs refuse queries answered from public
  resolvers), reported as `outbound25` / `ptr` / `resolver` checks
- Change-based health alerting: a worker re-runs the health-center
  checks on a schedule (default hourly) and announces only status
  transitions — a status must hold for two consecutive runs before it
  is announced, the first run records a baseline only, and recoveries
  are reported too. Delivery goes to the operations mailbox plus an
  optional JSON webhook (`MAILEZ_HEALTH_ALERT_WEBHOOK`,
  Slack/DingTalk/WeCom-compatible); `MAILEZ_HEALTH_ALERT_MUTE` silences
  noisy probes, `MAILEZ_HEALTH_ALERT=off` disables the worker
- IP ban engine: authentication failures count per source IP across the
  web login and the mail-proxy SASL gate; crossing `MAILEZ_BAN_MAX_RETRY`
  (default 20) within `MAILEZ_BAN_FINDTIME_SEC` (600) bans the address,
  with the duration escalating on its 30-day history (15 min up to 24 h).
  Loopback, the deployment's own addresses and `MAILEZ_BAN_WHITELIST`
  CIDRs are exempt, so probes can never ban themselves. The admin console
  gains a Bans page (list and lift, audited) and ban activity feeds the
  health center
- Scheduled encrypted backups: a daily worker (`MAILEZ_BACKUP_HOUR`)
  archives a consistent SQLite snapshot (`VACUUM INTO`) plus the upload
  tree, encrypts with chunked AES-256-GCM (`MAILEZ_BACKUP_KEY` — nothing
  runs without it, no plaintext archives) and publishes to a local
  directory (`MAILEZ_BACKUP_TARGET=local:<dir>`) or S3-compatible
  storage (`s3`), pruning beyond `MAILEZ_BACKUP_KEEP` (default 14). The
  admin Backups page shows configuration and run history, triggers a
  manual run and verifies archives (re-read + decrypt from the target);
  the health center grades backup freshness (warn at 7 days, fail at 14)

### Changed

- Engine `SORT` no longer opens one blob per message: sort keys are read
  from the header block the store caches at delivery (the listing already
  carried it), falling back to the blob only for messages that predate the
  cache. Sorting a 600-message folder used to spend tens of seconds opening
  blobs
- Deploy: operator override compose file, host bind knobs and service
  key passthrough for first-deployment hardening
- Dependency upgrades across the backend Go modules and the frontend
  workspace (Next.js 16.3.5, React 19.3.0)
- Bulk flag operations (mark-all-read, moving or deleting many messages)
  no longer cost a transaction per message: the engine commits a batch
  of flag changes in one write, and the control plane walks a large
  folder in windows instead of one pass. On a 300-message mailbox
  `UID STORE 1:* +FLAGS \Seen` went from ~34 s to ~7 s and
  `POST /mail/read-all` from ~30 s to ~14 s
- Domain reads are open to the domain's managers: `GET /domains`,
  `GET /domains/:name`, the DKIM status and the DNS wizard return the
  domains a manager holds, while every domain mutation (create, update,
  delete, key generation, managers, alternatives, relays) stays
  global-admin only

### Fixed

- Deleting a message while it was already in Trash did nothing: the API
  moved Trash → Trash, which the engine treats as a no-op, so the message
  stayed in the trash forever. `/mail/delete` now flags and UID-EXPUNGEs
  the message when the folder is Trash (and accepts a `uids` batch), and
  webmail's delete button inside Trash uses it
- Drive share links answered 401 to everyone without a session: the
  token-only download route sat inside the authenticated group, so the
  share token never had a chance. The route is public now (with optional
  session, so the owner still downloads without a token)
- `/api/v1/events` returned 500 on MySQL-backed deployments whenever the
  worker token had to be re-minted: `Save` wrote a zero `created_at`, which
  MySQL's strict mode rejects (SQLite accepted it). The row never updated,
  the freshly minted secret was never persisted, and every retry minted
  another Token row and failed the same way — wiping out SSE push and the
  outbox worker for that user
- The event watcher swallowed the first message that arrived right after a
  client subscribed: the first read was a pure baseline, so anything that
  landed between the client's own refresh and that read was baked into the
  baseline and never announced. The first check now announces the baseline
  (and runs as soon as the stream connects)
- Webmail's sign-in page logged a React hydration error (#418) on every
  visit: `passkeySupported()` was called during render, so the server
  emitted the form without the passkey button and the client emitted it
  with one. It is read through `useSyncExternalStore` with a server
  snapshot now
- An out-of-order SMTP command answered 502 ("Missing MAIL FROM command.");
  RFC 5321 §4.3.2 asks for 503. The plaintext SMTP listeners rewrite that
  reply (go-smtp v0.25.0 has no hook for it; encrypted streams are left
  byte-for-byte alone)
- Login rate limiting and the ban engine keyed trusted-proxy requests
  without a forwarded header into one empty-address bucket, so such
  callers shared a single limit; the socket address is used instead
- Deleting a contact that belongs to another user answered 204; it now
  answers 404
- Community compose shipped with the engine's mail ports (25/465/587/110/
  995/143/993/4190) bound to 127.0.0.1, so a fresh `mailezctl up ce`
  deployment could not receive external mail; they now default to
  `${MAILEZ_MAIL_BIND:-0.0.0.0}` while the debug ports keep the
  loopback default (`${MAILEZ_BIND:-127.0.0.1}`), matching the EE stacks
- Webmail no longer shows the admin console entry to non-admin accounts
- Web containers run in the operator's timezone
- `models.AutoMigrate` (the full-schema helper used by tests and tooling)
  was missing the health-snapshot, ban-record and backup-run tables;
  production migrations were unaffected
- External POP3 aggregation re-delivered every message on each poll: the
  delivered-UIDL list went to a column that does not exist, so the write
  failed silently and the cursor never advanced. The list is now stored
  in `seen_uid_ls`, and a failed cursor write is logged instead of
  dropped
- Deleting a user left the engine-side mailbox behind on the EE stacks:
  `MAIL_ENGINE_MGMT_ADDR` was passed without a URL scheme, so the purge
  request failed with `unsupported protocol scheme "mailezine"`; the
  compose files now carry `http://`
- Sorting a mailbox by sender, subject or size returned an empty page:
  the UIDs in an IMAP `SORT` response were read as integers while
  go-imap hands them back as strings, so the collector dropped every one
- A message containing a single very long line (a pasted URL or log
  line) failed with `mail service error`; bodies are now folded at the
  RFC 5321 line limit before submission
- A recipient over quota, or a message with an over-long line, produced
  a bare 502; both now answer 422 with `recipient_quota_exceeded` /
  `line_too_long` and a message the sender can act on
- Admin console requested its logo, branding and autoconfig XML from
  the site root, so all three 404'd on deployments that serve the
  console under `/admin`
- Webmail's service worker never registered (a TypeScript assertion had
  slipped into the shipped `sw.js`), which disabled offline support and
  the notification-click handler
- `POST /mail/send` ignored the `text` field that `POST /mail/draft`
  uses for the body, so a caller following the draft API sent an empty
  message; both field names are accepted
- Exchange ActiveSync was unreachable from the outside: the gateway
  template shipped no route for `/Microsoft-Server-ActiveSync`, so the
  mail hostname answered 404 and phones could only talk to the loopback
  backend port. `deploy/overrides/nginx/eas.conf` now publishes the
  endpoint on the mail hostname over 443 (EE)

## [1.0.0] - 2026-09-14

### Added

- Domain health center: `GET /admin/health` plus the admin "Health" page
  run per-domain live DNS checks (MX / SPF / DMARC / DKIM published-key
  comparison / Spamhaus ZEN blacklist / autoconfig / MTA-STS) and system
  probes (engine IMAP+MTA dial, Redis, database, disk, memory, cert
  expiry), each with ok/warn/fail/unknown and localized fix hints;
  Spamhaus PBL listings grade as advisories and 127.255.255.x error
  codes as probe errors rather than listings
- DNS wizard: `GET /domains/:name/dns-records` lists the nine records a
  deployment must publish with expected values and live verification;
  per-row wizard dialog on the domains page
- Client autoconfiguration endpoints: Mozilla autoconfig
  (`/mail/config-v1.1.xml` incl. `/.well-known/`), Microsoft
  autodiscover (XML + JSON), Apple mobileconfig profile and the MTA-STS
  policy file (`/.well-known/mta-sts.txt`, STSv1 mode `testing`), served
  through both frontends and the gateway
- Admin digest email (daily/weekly operations summary to the first
  admin) and the traffic report page with per-day aggregates, as
  optional modules
- TOTP self-enrollment: secret + `otpauth://` URI with verify-to-enable
  on the config page's Security tab
- Greylisting switch (`MAILEZINE_JUNK_GREYLIST`) exposed in the CE and
  EE compose files
- Gateway topology: the admin console is served under `/admin` on the
  webmail domain (basePath-aware), CardDAV/CalDAV route through
  `/dav/`, and the backend debug ports 8081–8083 bind to loopback only
  by default
- Webmail: per-message delete inside a conversation (undo restores just
  that message)
- Admin sidebar grouped by workflow: Overview & Monitoring /
  Organization / Mail Services / Compliance & Audit / System
- PostgreSQL control-plane support (`DB_DRIVER=postgres` alongside
  sqlite/mysql, GORM postgres driver): reserved-word `from`/`to` columns
  quoted per dialect via `clause.Column`, dialect-agnostic case-insensitive
  search (`LOWER(col) LIKE LOWER(?)`), portable `[]byte` mapping for raw
  message payloads; optional `--profile postgres` tier in the CE
  compose (external PostgreSQL/MySQL via `MAILEZ_DB_DRIVER`/
  `MAILEZ_DB_DSN` overrides); migration lock is MySQL-only for now —
  start one replica first when scaling on PostgreSQL

### Changed

- Admin console theme restored to the teal colour family (light `#2f8e6c`
  / dark `#3ba77f` primaries, teal accents, charts and sidebar)
- Community edition control plane defaults to SQLite (`./data/mailez.db`);
  MySQL stays available through the new `--profile mysql` compose tier
  (`MAILEZ_DB_DRIVER` / `MAILEZ_DB_DSN`)
- `mailezctl` manages the dev / community tiers as one
  compose project; raw compose invocations require
  `--env-file mailez.env` for the `${MAILEZ_STACK_SECRET:?}` interpolation
- First-login bootstrap via `mailez-seed` baked into the backend image
  (default `admin@example.com` / `MailezDemo2026!`, overridable with
  `MAILEZ_ADMIN_EMAIL` / `MAILEZ_ADMIN_PASSWORD`)

### Fixed

- Message parts are decoded through their declared charset (GBK and other
  legacy encodings render correctly instead of mojibake)
- External POP3/IMAP fetch disabled TLS verification (now verified by default;
  `FETCH_INSECURE` opts out)
- Email HTML rendering was vulnerable to stored XSS; sanitized on both sides
- Web login had no brute-force protection (rate limited)
- Session cookies were not marked Secure; `COOKIE_SECURE` now controls it
- Default `SECRET_KEY` is rejected in production mode
- CardDAV/CalDAV cross-account IDOR: the `{user}` path segment is now
  checked against the authenticated identity at the router (a valid DAV
  credential could previously read/overwrite/delete any account's contacts
  and calendars); the shared WebDAV handler was also instantiated per
  request, removing a backend data race
- `/mail/merge` bypassed the send-identity policy (`MaySendAs`) that
  `/mail/send` enforces — any user could mail-merge as any address
- `/mail/labels` trusted `X-Delegate-Email` without validating the
  delegation grant
- User deletion now purges dependent rows and drive/upload blobs first
  (MySQL RESTRICT foreign keys made it fail; SQLite orphaned rows)
- Fetch poller uses dial/read deadlines (one dead remote no longer stalls
  every other account's polling); engine-account purge uses an HTTP timeout
- Production weak-secret guard extended to `MAILEZ_STACK_SECRET` (empty
  secret would leave the internal `/stack` API unauthenticated); prod
  composes set `MAILEZ_ENV=production` so the guard can fire
- Metrics listener defaults to loopback (`127.0.0.1:9090`)
- SQLite control plane runs WAL + a 10s busy timeout (background writers
  no longer surface "database is locked" under load)
- Per-email login lockout is enforced (previously counted but never acted
  on); exceeding it now returns 429 before the bcrypt check
- Webmail: account/delegate switch re-scopes API calls before loading the
  mailbox (previously showed the previous account's mail); AI search and
  the snoozed view take the same sequence guards as keyword search; the AI
  streaming compose interval is cleared on every exit path
- Admin: save/create errors in the six list pages (users, domains, aliases,
  relays, tokens, fetches) were written to a state that only rendered inside
  the (closed) dialog — a failed save silently did nothing; errors now show
  inside the open dialog (cleared on reopen) while load/delete errors render
  on the page
- Webmail: message rows no longer re-render on every store change — the
  memoized row subscribed to the whole mailbox store context for label
  colors; the color map now lives in its own context whose identity changes
  only when label definitions change
- Webmail: the compose rich-text toolbar was hardcoded Chinese regardless
  of locale (titles, font-family labels, the size dropdown); every control
  is now localized and gained a tooltip (24 new `mail.editor` keys per
  locale)
- Public self-signup is rate limited per IP (10/hour, shared store counter
  with login limiting) — an unauthenticated write endpoint previously let
  a bot provision accounts at line rate
- Mailezine: `EXAMINE` is now actually read-only — STORE/COPY/MOVE/EXPUNGE
  and APPEND into the examined mailbox return `NO`, FETCH body sections no
  longer implicitly set `\Seen`, CLOSE degrades to UNSELECT, and the
  selection reports no permanent flags (previously EXAMINE was fully
  writable, so clients that open mailboxes read-only could mutate them)
- Mailezine: PROXY protocol v1 headers are only honored from trusted peers
  (new `MAILEZINE_PROXY_TRUSTED`, default loopback/private CIDRs) — a
  client that reached a PROXY-enabled port directly could previously forge
  the client IP that relay/auth decisions trust; the header read is also
  bounded (30s deadline, 128-byte line cap) so a silent or padding peer
  cannot hold connections open
- Mailezine: outbound multi-recipient delivery semantics are now pinned as
  at-least-once (per-recipient queue state, retries only pending
  recipients): a connection lost after DATA may duplicate on retry —
  documented at the failure point and in DECISIONS D47 instead of being an
  undocumented surprise
- e2e CI seeds via in-container `mailez-seed` (matches the SQLite default)
- Website: EN locale links keep the `/en` prefix (navbar and page CTAs
  previously navigated back to the Chinese pages); EN Mailezine page says
  two counter-balanced benchmark rounds (not three); pricing/compare table
  wording fixed and SQLite named as the community control plane; migration
  FAQ no longer promises incremental re-runs

### Removed

- Dead env entries `MAILEZ_RELAYHOST` / `MAILEZ_REJECT_UNLISTED_RECIPIENT`
  / `MAILEZ_FTS` from `mailez.env.example` (nothing read them)
- Build-time configuration switches for optional capabilities: the tree
  now builds a single community configuration; optional capabilities
  render as visible locked entry points until their module is enabled.
  `mailezctl` covers the dev / ce targets; the installer provisions the
  ce compose profile

## [1.0.0-rc.1] - 2026-08-28

### Added

- Web Push notifications: VAPID endpoint, subscription API, background
  notifier polling unseen counts for subscribed users
- Prometheus metrics on a dedicated port (`MAILEZ_METRICS_ADDR`, default :9090)
- Versioned database migrations (`schema_migrations` table)
- Login rate limiting (per IP and per account) and TOTP brute-force protection
- Frontend engineering scaffold: ESLint, Prettier, Vitest and CI jobs for
  webmail and admin
- Shared API contract types in `frontend/packages/types`
- Meeting invitations (iTIP): parse REQUEST/REPLY/CANCEL, one-click
  accept/decline/tentative from the mail reader and the calendar, send new
  invitations with attendees
- Attachment full-text search in mailezine (PDF/OOXML/ODF/text extraction
  with Tika fallback) for the mailbox search
- Internal `/stack` API authentication via `MAILEZ_STACK_SECRET`
  (`X-Stack-Secret` header) used by the mail agent and the mailezine engine
- Read receipts (RFC 3798): request a receipt when composing, answer with a
  disposition notification from the reader, `$MDNSent` dedupe keyword
- Same-system mail recall: recall a sent message from Sent Items (classic-style `X-MS-Recall` notices) and apply a recall on the reader side
- Mail merge (逐封群发): paste recipient rows with `{{name}}`/`{{email}}`/
  custom variables; one personalized copy per recipient
- Folder-level "mark all read" (`/mail/read-all`)
- Calendar sharing between accounts (read-only or read-write), event
  reminders delivered as mail before start (also triggers web push), and a
  subscribable ICS feed with token-protected export
- Webmail sends now keep a copy in Sent Items (the engine does not
  auto-copy submissions), which also powers recall
- Smart Reply (智能回复): 2-3 ready-to-send reply suggestions generated by
  the AI provider, inserted into the inline quick-reply box with one click
- Download every attachment of a message as a zip (`/mail/attachments/zip`)
- Reply with selected text: select a passage in the reader and reply quoting
  only that passage (mainstream-style)
- Large-attachment relay (超大附件): files over the inline cap are uploaded
  to a token-protected server store and mailed as a download link, with a
  rolling 30-day expiry cleaned by a background worker
- Burn-after-read (阅后即焚): compose with an expiry window; the reader
  reveals the body once, flags `$BurnRead` and overlays a viewer watermark
- Login security alerts: a successful sign-in from a previously unseen IP
  mails the account owner a new-device notice
- Cloud drive (云盘): per-user file/folder tree with a pluggable blob store
  (local disk or MinIO via `MAILEZ_DRIVE_BACKEND=minio`), upload/list/
  download/rename/move/trash/restore, share links and an empty-trash worker
- Admin system overview (`/admin/overview` + dashboard page): users/domains/
  aliases, drive and large-attachment storage usage, engine/host identity
- PWA/offline: installable web app manifest, apple-web-app metadata and
  service-worker precache of the app shell for faster offline startup

### Changed

- Backend reorganized from a flat `api` package into domain packages
  (`user`, `domain`, `alias`, `mailbox`, `compose`, `contacts`, `sieve`,
  `admin`, `fetch`, `push`, `ai`, `stack`); `mail.Client` is now behind the
  `mail.Gateway` interface so handlers are testable
- Mail-stack service renamed `mail-store` → `mail-keeper`; compose project
  name pinned to `mailez` (containers `mailez-*`)
- HTTP hardening: request/response timeouts, 64 MiB body limit, CORS
  allow-list, request-id and access logging
- Error responses no longer leak internals; 5xx map to generic messages
- Mail HTML is sanitized server-side (bluemonday) and client-side (DOMPurify)
- Graceful shutdown on SIGINT/SIGTERM; background workers cancel cleanly

### Fixed

- `mail.received` webhooks only fired for users with a browser push
  subscription; webhook-only users never received events (notifier now
  polls webhook users with a per-webhook token)
- Opening a saved draft had no edit action and sending it left the draft
   behind (`mailDelete` was not wired into the send flow)
- Snoozed messages were not hidden from the inbox (the displayMessages
  filter was never consumed by the list), and the internal `$Snoozed*`/
  `$Muted`/`$Pin` keywords surfaced as user labels case-sensitively
- Snooze keywords were matched case-sensitively across the stack while
  go-imap canonicalizes keywords to lowercase, so wake-up times were lost
  (until became "now+365d") and unsnoozing never stripped the flag
- Search builder: conditions persisted across dialog opens (leaking stale
  filters into saved searches) and Apply dropped an in-progress condition;
  Apply also stayed disabled while editing the first condition
- A slow folder load could overwrite a fresh search result (search now
  invalidates in-flight folder loads)
- The announcement banner was a fixed z-50 overlay that covered the search
  box; it now flows with the layout
- Sending with the undo window never refreshed the inbox after delivery
- The static `manifest.webmanifest` collided with the generated route (500)
- Multipart message builder emitted a duplicate boundary, corrupting the
  first body part when attachments were present
- Attachments larger than 4 MiB were rejected by the default HTTP body limit
