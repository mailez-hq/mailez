# mailez deployment scripts

## `mailezctl` — unified stack management

`deploy/mailezctl.ps1` (Windows) / `deploy/mailezctl.sh` (Linux/macOS) is the
single entry point for the whole stack. Three self-contained compose files
cover development plus the two production editions:

| File                                    | Edition      | Engine           | Storage                  |
| --------------------------------------- | ------------ | ---------------- | ------------------------ |
| `docker-compose.dev.yml`                | Development  | mailezine        | SQLite + Pebble + local FS |
| `docker-compose.ce.yml`          | Community    | mailezine        | SQLite (default; MySQL optional) + Pebble + local FS |
| `docker-compose.ee.yml`         | Enterprise   | mailezine        | MySQL + TiDB + MinIO/S3  |

All files belong to one compose project (`mailez`), so `ps`/`logs`/`down`
manage the same stack whichever edition is active.

```sh
./deploy/mailezctl.sh up                    # dev: SQLite + Pebble + local FS
./deploy/mailezctl.sh up ce          # community: mailezine + SQLite (default)
./deploy/mailezctl.sh up ee         # enterprise: mailezine + MySQL + TiDB + MinIO/S3
./deploy/mailezctl.sh ps                    # status (same project regardless)
./deploy/mailezctl.sh logs ee mailezine -Follow
./deploy/mailezctl.sh down
```

The dev tier expects the backend on the host at `:8080` (see
`docs/dev-setup.md`); both production editions are fully containerized.
Switching editions does not migrate existing mail — treat the edition as a
deployment-time choice. Engine-only development (no control plane) uses the
scripts in the mailezine repo (`deploy/scripts/tidb-dev.ps1`,
`tidb-storage-e2e.ps1`).

## `go run ./cmd/e2e` — end-to-end mail path smoke test

Verifies, against a running `docker compose up` stack, that the whole mail
path works: backend health, admin login, domain/user provisioning, DKIM key
generation, authenticated SMTP submission, IMAP delivery, DKIM-Signature and
rspamd spam headers, and (optionally) alias delivery.

Prerequisites:

- The stack is up (`mailezctl` passes `--env-file mailez.env` for you):
  `cd deploy && ./mailezctl.sh up ee`
  (local source build: `MAILEZ_LOCAL_BUILD=1 ./mailezctl.sh up ee`)
- The backend has been seeded once (creates `admin@example.com` /
  `MailezDemo2026!`). Easiest from a container:

  ```sh
  cd deploy && docker compose --env-file mailez.env -f docker-compose.ee.yml exec backend mailez-seed
  ```

  For a host-side seed, point `DB_DRIVER`/`DB_DSN` at the deployed database
  (the community default is SQLite inside the container at `/data/mailez.db`).
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
