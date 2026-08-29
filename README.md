<p align="center">
  <img src="branding/mailez-logo.svg" alt="mailez" width="320">
</p>

# mailez — mail easy

Self-hosted email that scales with you: from a personal mailbox of one to
ten-thousand-person organizations — teams, enterprises and public-sector
agencies alike. Send and receive mail with your own domain, keep your data in
your own hands, and enjoy a webmail that actually feels good to use. One
command deploys the whole thing — mail delivery, spam filtering,
authentication, admin console and webmail.

## Who is it for?

- **Individuals** — a mailbox that is truly yours, not rented from a mail provider
- **Businesses and organizations** — domain mail for everyone, with data kept 100% in-house
- **Public sector** — strict data-sovereignty and privacy requirements, fully covered

The common ground: self-hosted and privacy-first, with no Linux mail expert
required to run it.

## What you get

### Webmail that feels like a native app

- **Live, not refreshed** — new mail arrives over a real-time push channel, so
  the inbox updates itself; reading, replying and organizing never reload the
  page, and long lists scroll smoothly through 10,000+ messages
- **Three-pane layout** — folders, list and reading pane side by side; the
  list column is draggable to your preferred width, and the reading pane
  collapses quoted replies Gmail-style so long threads stay scannable
- **Search that just works** — type naturally (`from:`, `to:`, `has:attachment`,
  dates…), save frequent searches, and filter with one click for unread,
  starred or messages with attachments
- **Keyboard-first** — press `/` to search, `⌘K` for the command palette, `?`
  for the full shortcut list
- **Day-to-day mail tasks made easy** — conversation threads, quick reply and
  AI summary right in the action bar, snooze, scheduled send, undo toast for
  bulk move / archive / delete, and drafts that keep every recipient including
  Bcc
- **An AI assistant (optional)** — summarize long threads, draft replies in
  the tone you want, auto-prioritize your inbox, or search by meaning instead
  of keywords
- **A workbench, not just an inbox** — the home dashboard surfaces recent
  files and upcoming events, both clickable straight into context
- **Privacy features built in** — PGP sign and encrypt (distinct from your
  personal signature, with an optional auto-signature), two-factor
  authentication, remote-image blocking, a Sieve filter editor, and contacts
  grouped by sender
- **Works offline** — installs as a PWA; light/dark themes and three list
  densities for comfort

### Mail is just the start

- **Calendar** — events with reminders, shared calendars, and subscriptions
  you can plug into Apple Calendar or Google Calendar via a private ICS link
- **Contacts** — vCard import/export, duplicate merging, and CardDAV sync for
  phones
- **Drive** — upload and organize files, share by link, restore from trash;
  recent files show up on the dashboard

### Any device, any client

- **Standard protocols** — SMTP / IMAP / POP3 (implicit TLS available), plus
  CardDAV / CalDAV and Exchange ActiveSync for phone sync; Thunderbird,
  Outlook and Apple Mail configure themselves via autoconfig/autodiscover
- **Delegated mailboxes** — grant a teammate full access to your mailbox (or
  manage a shared one) without sharing passwords
- **App tokens** — per-client tokens you can issue and revoke from settings

### An admin console that doesn't feel like admin work

- **Manage everything in one place** — domains, mailboxes, aliases, relays,
  external mailbox fetching and app tokens; deleting a user cleans up their
  engine-side mailbox automatically
- **One-click DKIM** — generate signing keys with a status hint, so your mail
  stops landing in spam
- **See who did what** — audit log of admin actions, role-based access
  (admin / manager / user), and a site-wide announcement banner
- **Backup or migrate easily** — export and import your whole configuration

### Trust and security under the hood

- **Spam filtering that works** — Rspamd learns from your reporting; DKIM /
  DMARC / ARC signing and checking keep your mail deliverable
- **Transport security** — MTA-STS and DANE protect mail in transit;
  per-mailbox quotas and sending rate limits keep the system healthy
- **Malware scanning & content controls** — attachments are scanned for
  macros and known threats; outbound DLP and compliance archiving are
  available on the enterprise edition

## Quick start

Three self-contained deployment tiers ship as compose files, managed by one
entry point:

| Edition | Engine | Storage |
|---|---|---|
| **dev** (default) | mailezine | SQLite + Pebble + local FS |
| **community** | mailezine | MySQL + Pebble + local FS |
| **enterprise** | mailezine | MySQL + TiDB + MinIO/S3 |

```sh
./deploy/mailezctl.sh up              # dev tier
./deploy/mailezctl.sh up community    # community edition (production)
./deploy/mailezctl.sh up enterprise   # enterprise edition (production)
```

The dev tier expects the backend on the host at `:8080` (build the images
once with `cd backend && go run ./cmd/build-images`; details in
[`docs/dev-setup.md`](docs/dev-setup.md)). Both production editions are fully
containerized and publish:

| Port | What's there |
|---|---|
| http://localhost:8082 | Admin console |
| http://localhost:8083 | Webmail |
| http://localhost:8081 | Backend API (for developers) |
| 25/465/587/143/993/4190 … | Mail protocols (SMTP / IMAP / ManageSieve) |

TLS is off by default for local testing. For production, follow
[`deploy/certs/README.md`](deploy/certs/README.md) to enable automatic
certificates.

After the stack is up, verify the whole mail path works end to end:

```sh
cd backend
go run ./cmd/seed   # creates the admin account once
go run ./cmd/e2e    # sends a test mail, checks delivery, DKIM and spam filtering
```

## Tech stack (for developers)

- Backend: Go + Fiber, GORM, Redis
- Frontend: Next.js (React) — separate admin and webmail apps
- Mail engine: **mailezine** (a single Go binary) speaks
  SMTP/IMAP/POP3/ManageSieve behind an engine-agnostic directory contract
  (`/stack/directory/*`) with pluggable KV + blob storage — every tier
  (dev, community, enterprise) runs it; the tiers differ only in storage
  scale (SQLite/pebble vs MySQL/TiDB/MinIO) and licensed features
- More details: [`docs/dev-setup.md`](docs/dev-setup.md),
  [`docs/architecture.md`](docs/architecture.md),
  [`docs/webmail-ui-spec.md`](docs/webmail-ui-spec.md)

## License

[mailez License](LICENSE) — Apache License 2.0 with additional use conditions:

- **No SaaS** — may not be provided to third parties as a hosted/managed/SaaS offering
- **Own-use only** — use is limited to operating mail services for yourself or your
  organization, wherever it is deployed (on-premises, private cloud, or public cloud)
- **No third-party multi-tenant service** — a single deployment may not serve multiple
  independent organizations; running multiple domains or mailboxes for your own
  organization is fine
