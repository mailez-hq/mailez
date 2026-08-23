# mailez architecture

## Overview

mailez is a self-hosted mail platform built from three layers:

1. **Mail images (Docker)** — nginx gateway, Postfix (MTA), Dovecot
   (mail-keeper), Rspamd (mail-filter), macro-scanner, Unbound
   (resolver), Redis. All components are self-built images driven by a single
   Go agent binary (`mailez-agent`).
2. **Backend (Go)** — the control plane: REST API for admin/webmail, the
   internal API the mail images authenticate against, SSO sessions, Sieve,
   fetching, push notifications, AI.
3. **Frontends (Next.js)** — `webmail` and `admin` apps, both consuming the
   same `/api/v1` contract.

## Backend layout

The backend is organized into **domain packages**, each owning its HTTP
handlers and business rules, with a single composition root:

```
backend/internal/
├── core/          config, shared App (DB/Auth/Cfg/Mail), middleware, models
├── auth/          sessions, SSO, TOTP, login rate limiting
├── user/          user CRUD, profile, password, 2FA, PGP, app tokens
├── domain/        domains, DKIM, managers, alternatives, relays
├── alias/         aliases, anonymous aliases, send-as identities
├── mailbox/       folder/list/detail/search/thread/move/flag (read path)
├── compose/       send + drafts (write path, attachments, Cc/Bcc)
├── contacts/      address book
├── sieve/         filter rules
├── admin/         audit trail, config export/import
├── fetch/         external POP3/IMAP polling
├── push/          Web Push subscriptions + notifier
├── ai/            LLM provider abstraction + AI endpoints
├── stack/         internal API contract for nginx/postfix/dovecot/rspamd
├── mail/          IMAP/SMTP gateway behind the mail.Gateway interface
└── server/        composition root: Fiber app, migrations, background workers
```

Dependency direction is one-way: domains depend on `core` (and infrastructure
packages such as `mail`); `server` depends on everything and nothing depends on
it. This keeps the graph acyclic and lets handler tests inject a fake
`mail.Gateway` instead of live mail services.

## Authentication flow

- Web login issues an HTTP-only session cookie (Redis-backed, sliding TTL).
- Every webmail request exchanges the session for a short-lived `token-*`
  temporary credential used as the IMAP/SMTP password — the user's real
  password never reaches the browser.
- The mail images authenticate through `/stack/*`: nginx's mail proxy and
  Dovecot's passdb call the backend, which validates credentials, 2FA state,
  protocol permissions and rate limits.

## Email HTML safety

Untrusted message HTML is sanitized twice: server-side by bluemonday when the
message body is parsed, and client-side by DOMPurify immediately before
rendering. Remote images stay blocked until the reader opts in.

## Frontend layout

Both apps mirror the backend domains under `components/` (`mailbox/`,
`compose/`, `contacts/`, `sieve/`, `settings/`, `palette/`). The webmail keeps
mailbox state in a `MailStore` context provider with `MailView` as a thin
render shell; routes are URL-driven (`/mail/[folder]/[id]`).

Wire types shared by both apps live in `frontend/packages/types`. The roadmap
is to generate this package from the backend OpenAPI document.

## Verification

- `make verify` runs gofmt/vet/tidy/tests (backend) and typecheck/lint/tests
  (frontends)
- `make e2e` runs the end-to-end smoke test against the dev stack
- CI enforces the same checks for every push and pull request
