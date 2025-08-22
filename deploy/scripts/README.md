# mailez deployment scripts

## `go run ./cmd/e2e` — end-to-end mail path smoke test

Verifies, against a running `docker compose up` stack, that the whole mail
path works: backend health, admin login, domain/user provisioning, DKIM key
generation, authenticated SMTP submission, IMAP delivery, DKIM-Signature and
rspamd spam headers, and (optionally) alias delivery.

Prerequisites:

- The stack is up: `cd deploy && docker compose up -d --build`
- The backend has been seeded once (creates `admin@example.com` /
  `MailezDemo2026!`). Run the seed from a container or locally:

  ```sh
  cd backend && go run ./cmd/seed   # uses ./mailez.db next to it
  ```

  or point `DB_DSN` at the deployed database.
- Go toolchain (the command lives in the backend module and reuses `go-imap`).

Usage:

```sh
cd backend
go run ./cmd/e2e --domain e2e.example.com --alias team
```

Common options: `--host`, `--api-port` (default 8081), `--smtp-port`
(default 587), `--imap-port` (default 143), `--admin-email` /
`--admin-password`, `--domain`, `--user`, `--alias <localpart>`, `--lenient`
(report DKIM/spam header issues without failing).

The command exits non-zero on any failed check. With `TLS_FLAVOR=notls`,
STARTTLS is attempted first and plaintext is used as fallback, so both dev and
TLS-enabled deployments are covered.

## `generate-certs.*` — self-signed certificates for local TLS testing

See [`deploy/certs/README.md`](../certs/README.md).
