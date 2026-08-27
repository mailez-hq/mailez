# mailez deployment scripts

## `mailezctl` — unified stack management

`deploy/mailezctl.ps1` (Windows) / `deploy/mailezctl.sh` (Linux/macOS) is the
single entry point for the whole stack. All compose files belong to one
compose project (`mailez`), so `ps`/`logs`/`down` always manage the same
stack no matter which engine profile is active.

Storage tiers:

| Tier   | Control plane | Engine KV | Message blob | Engine profile            |
| ------ | ------------- | --------- | ------------ | ------------------------- |
| Dev    | SQLite (host) | Pebble    | local FS     | `mailezine`               |
| Dev    | SQLite (host) | TiDB      | local FS     | `mailezine-tidb`          |
| Prod   | MySQL         | Pebble    | MinIO/S3     | `mailezine` + `-Prod`     |
| Prod   | MySQL         | TiDB      | MinIO/S3     | `mailezine-tidb` + `-Prod`|

`postdove` (postfix + dovecot) remains available for both tiers; the prod
base always adds `docker-compose.mysql.yml`.

```sh
./deploy/mailezctl.sh up mailezine-tidb     # dev: TiDB KV + local FS blob
./deploy/mailezctl.sh up mailezine          # dev: Pebble KV + local FS blob
./deploy/mailezctl.sh up postdove           # classic postfix + dovecot
MAILEZCTL_PROD=1 ./deploy/mailezctl.sh up mailezine-tidb   # prod: MySQL + TiDB + MinIO
./deploy/mailezctl.sh ps                    # status (same project regardless)
./deploy/mailezctl.sh logs mailezine -Follow
./deploy/mailezctl.sh down
```

`-Prod` (ps1) / `MAILEZCTL_PROD=1` (sh) switches the base file to
`docker-compose.yml` (fully containerized backend/frontends) and layers on
`docker-compose.mysql.yml` (control plane MySQL) plus
`docker-compose.blob-s3.yml` for engine profiles (MinIO/S3 blob); the dev
base expects the backend on the host at `:8080` (see `docs/dev-setup.md`).

Engine/storage notes:

- `mailezine` runs the pure-Go engine with Pebble KV; blob is local FS in
  dev and MinIO/S3 in prod.
- `mailezine-tidb` keeps the same engine but points its KV layer at the
  single-node TiDB service added by `docker-compose.tidb.yml` (blob follows
  the tier). Switching KV backend does not migrate existing mail; treat it
  as a deployment-time choice.
- The mailezine repo keeps an engine-only dev/e2e stack
  (`mailezine/deploy/docker-compose.tidb.yml`) for backend-free development;
  the unified product stack above is the deployment surface.

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

The command exits non-zero on any failed check. With `MAILEZ_TLS=off`,
STARTTLS is attempted first and plaintext is used as fallback, so both dev and
TLS-enabled deployments are covered.

## `generate-certs.*` — self-signed certificates for local TLS testing

See [`deploy/certs/README.md`](../certs/README.md).
