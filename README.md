<p align="center">
  <img src="branding/mailez-logo.svg" alt="mailez" width="320">
</p>

# mailez — mail easy

Self-hosted email for teams who want their own mail server: send and receive
mail with your own domain, keep your data in your own hands, and enjoy a
webmail that actually feels good to use. One command deploys the whole thing —
mail delivery, spam filtering, authentication, admin console and webmail.

## Who is it for?

- Teams and companies that want their own domain mail without handing their
  data to third-party mail services
- Anyone who wants a self-hosted, privacy-first alternative that doesn't
  require a Linux mail expert to run

## What you get

### Webmail that feels like a native app

- **Three-pane layout** — folders, list and reading pane side by side; read,
  reply and organize mail without page reloads; the list column is even
  draggable to your preferred width
- **Search that just works** — type naturally (`from:`, `to:`, `has:attachment`,
  dates…), save frequent searches, and filter with one click for unread,
  starred or messages with attachments
- **Keyboard-first** — press `/` to search, `⌘K` for the command palette, `?`
  for the full shortcut list; long lists scroll through 10,000+ messages
  without a stutter
- **Day-to-day mail tasks made easy** — conversation threads, bulk move /
  archive / delete with an undo toast, pull-to-refresh for new mail
- **An AI assistant (optional)** — summarize long threads, draft replies in
  the tone you want, auto-prioritize your inbox, or search by meaning instead
  of keywords
- **Privacy features built in** — PGP sign and encrypt, two-factor
  authentication, remote-image blocking, a Sieve filter editor, and a contact
  view grouped by sender
- **Works offline** — installs as a PWA; light/dark themes and three list
  densities for comfort

### An admin console that doesn't feel like admin work

- **Manage everything in one place** — domains, mailboxes, aliases, relays,
  external mailbox fetching and app tokens
- **One-click DKIM** — generate signing keys with a status hint, so your mail
  stops landing in spam
- **See who did what** — audit log of admin actions, role-based access
  (admin / manager / user)
- **Backup or migrate easily** — export and import your whole configuration

### Trust and security under the hood

- **Spam filtering that works** — Rspamd learns from your reporting; DKIM /
  DMARC / ARC signing and checking keep your mail deliverable
- **Transport security** — MTA-STS and DANE protect mail in transit;
  per-mailbox quotas and sending rate limits keep the system healthy
- **Malware scanning** — attachments are scanned for macros and known threats

## Quick start

Requires Docker (Compose v2) and a Go toolchain (1.22+) for the one-time
image build.

```sh
cd backend
go run ./cmd/build-images

cd ../deploy
cp mailez.env.example mailez.env   # set MAILEZ_SECRET_KEY, MAILEZ_DOMAIN, MAILEZ_HOSTNAMES
docker compose up -d --build
```

The images are tagged `mailez/*:local` and referenced by the compose files,
so nothing is pulled from a public registry. `build-images` discovers
components from the directory layout (`deploy/images` for shared services,
`deploy/engines/<engine>` for engine-specific ones), so adding a new engine
needs no script changes.

| Port | What's there |
|---|---|
| http://localhost:8082 | Admin console |
| http://localhost:8083 | Webmail |
| http://localhost:8081 | Backend API (for developers) |
| 25/587/143/993/4190 … | Mail protocols (SMTP / IMAP / ManageSieve) |

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
- Mail engine (pluggable, default `postdove`): Postfix + Dovecot behind an
  nginx gateway, with Rspamd filtering and Unbound DNS; the control plane
  talks to the engine through an engine-agnostic directory contract
  (`/stack/directory/*`), so a second engine (e.g. Stalwart) can slot in
  behind the same API
- More details: [`docs/dev-setup.md`](docs/dev-setup.md),
  [`docs/webmail-ui-spec.md`](docs/webmail-ui-spec.md)

## License

[mailez License](LICENSE) — Apache License 2.0 with additional use conditions:

- **No SaaS** — may not be provided to third parties as a hosted/managed/SaaS offering
- **Own-use only** — use is limited to operating mail services for yourself or your
  organization, wherever it is deployed (on-premises, private cloud, or public cloud)
- **No third-party multi-tenant service** — a single deployment may not serve multiple
  independent organizations; running multiple domains or mailboxes for your own
  organization is fine
