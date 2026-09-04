# Changelog

All notable changes to mailez are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and adheres to
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- PostgreSQL control-plane support (`DB_DRIVER=postgres` alongside
  sqlite/mysql, GORM postgres driver): reserved-word `from`/`to` columns
  quoted per dialect via `clause.Column`, dialect-agnostic case-insensitive
  search (`LOWER(col) LIKE LOWER(?)`), portable `[]byte` mapping for raw
  message payloads; optional `--profile postgres` tier in the CE
  compose (external PostgreSQL/MySQL via `MAILEZ_DB_DRIVER`/
  `MAILEZ_DB_DSN` overrides); migration lock is MySQL-only for now —
  start one replica first when scaling on PostgreSQL

### Changed

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
- Same-system mail recall: recall a sent message from Sent Items (Outlook-
  style `X-MS-Recall` notices) and apply a recall on the reader side
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
  only that passage (Gmail/Outlook-style)
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
