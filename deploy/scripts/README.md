# mailez deployment scripts

## `mailezctl` — unified stack management

`deploy/mailezctl.ps1` (Windows) / `deploy/mailezctl.sh` (Linux/macOS) is the
single entry point for the whole stack. There are exactly two self-contained
compose files, one per storage tier:

| File                            | Control plane | Engine KV | Message blob |
| ------------------------------- | ------------- | --------- | ------------ |
| `docker-compose.dev.yml`        | SQLite (host) | Pebble    | local FS     |
| `docker-compose.prod.yml`       | MySQL         | TiDB      | MinIO/S3     |

Both files belong to one compose project (`mailez`), so `ps`/`logs`/`down`
manage the same stack either way.

```sh
./deploy/mailezctl.sh up                    # dev: SQLite + Pebble + local FS
./deploy/mailezctl.sh up prod               # prod: MySQL + TiDB + MinIO/S3
./deploy/mailezctl.sh ps                    # status (same project regardless)
./deploy/mailezctl.sh logs prod mailezine -Follow
./deploy/mailezctl.sh down
```

The dev tier expects the backend on the host at `:8080` (see
`docs/dev-setup.md`); the prod tier is fully containerized. Switching tiers
does not migrate existing mail — treat the tier as a deployment-time choice.
The mailezine repo keeps an engine-only dev/e2e stack
(`mailezine/deploy/docker-compose.tidb.yml`) for backend-free development.

## `go run ./cmd/e2e` — end-to-end mail path smoke test

Verifies, against a running `docker compose up` stack, that the whole mail
path works: backend health, admin login, domain/user provisioning, DKIM key
generation, authenticated SMTP submission, IMAP delivery, DKIM-Signature and
rspamd spam headers, and (optionally) alias delivery.

Prerequisites:

- The stack is up: `cd deploy && docker compose -f docker-compose.prod.yml up -d --build`
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

The command exits non-zero on any failed check. With `MAILEZ_TLS=off`,
STARTTLS is attempted first and plaintext is used as fallback, so both dev and
TLS-enabled deployments are covered.

## `generate-certs.*` — self-signed certificates for local TLS testing

See [`deploy/certs/README.md`](../certs/README.md).
