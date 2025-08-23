# Security

## Reporting a vulnerability

Please do **not** open a public issue for security problems. Report privately to
the maintainers (mailto:security@mailez.invalid — replace with the real
address before release) and include:

- affected version / commit
- a minimal reproduction
- impact assessment if known

You should receive an acknowledgement within 72 hours. We follow a 90-day
disclosure window for confirmed issues unless the reporter agrees otherwise.

## Security posture

- **Authentication** — PBKDF2-SHA256 password hashing; per-IP and per-account
  login rate limiting; TOTP second factor; HTTP-only, SameSite session cookies
  (Secure flag via `COOKIE_SECURE`); per-session temporary tokens for IMAP/SMTP
  access so the mailbox password is never exposed to the web client.
- **Injection** — all SQL is parameterized through GORM; email HTML is
  sanitized server-side (bluemonday) and client-side (DOMPurify) before render.
- **Transport** — opportunistic TLS with verification for external fetch
  accounts (`FETCH_INSECURE` is an explicit opt-out); MTA-STS and DANE for
  outbound SMTP.
- **Secrets** — application secrets are environment-driven; the server refuses
  to start in `MAILEZ_ENV=production` with the default `SECRET_KEY`; reversible
  credentials (fetch passwords, push tokens) are encrypted at rest with
  AES-GCM keyed from `SECRET_KEY`.
- **HTTP hardening** — request timeouts, bounded body size, permissive CORS is
  replaced by an explicit allow-list (`CORS_ORIGINS`), Prometheus metrics on a
  dedicated port.

## Supported versions

Only the latest release on `main` receives security fixes. Backports are made
on request for the previous minor release.
