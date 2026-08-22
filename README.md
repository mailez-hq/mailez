# mailez

Self-hosted mail server control plane, rewritten in Go + Next.js.

Self-hosted mail stack built on proven third-party components (nginx / Postfix
/ Dovecot / Rspamd) with a fully self-developed control plane (admin, SSO,
webmail) in Go + Next.js.

## Stack

- Backend: Go + Fiber, GORM (SQLite dev / MySQL prod), Redis
- Frontend: Next.js App Router + React, shadcn/ui
- Webmail: self-built IMAP client (go-imap) with optional AI plugin layer
- License: Apache 2.0

## Layout

```
backend/    Go control plane (admin API, internal API, SSO, configgen, podop, mail gateway, ai)
frontend/   Next.js apps (apps/admin, apps/webmail)
deploy/     Docker Compose (nginx/postfix/dovecot/rspamd + backend/frontend)
branding/   logo assets (SVG)
```

## Status

Active development. Implemented so far:

- Go control plane: models, SSO sessions, REST API v1 (roles + audit), app tokens
- Internal API consumed by the mail stack (vendored podop bridge): postfix maps,
  dovecot passdb/userdb/quota/sieve, nginx auth, client autoconfig, SRS, rate limit
- Admin UI (domains / users / aliases / relays / fetches / tokens / audit / config)
- Self-built webmail: IMAP/SMTP gateway with temp-token auth, three-pane UI,
  optional AI provider layer (summarize / draft)

The internal API wire contract is pinned by contract tests in
`backend/internal/internalapi` so upgrades of the mail-stack images cannot break
the integration silently.

Deployment: `deploy/mailez.env` + `deploy/docker-compose.yml` wire the
self-built mail-stack images (nginx/postfix/dovecot/rspamd, built from
`deploy/vendor/mailstack` via `deploy/scripts/build-images.ps1`) to the mailez
backend internal API at `http://backend:8080` (the port the mail-stack images
hardcode). The backend listens
on 8080 in-container, published as `127.0.0.1:8081`. The admin and webmail
Next.js apps run as their own services, proxying `/api/v1` to the backend via
`API_TARGET=http://backend:8080`, and are published as `127.0.0.1:8082`
(admin, basePath `/admin`) and `127.0.0.1:8083` (webmail). The front
nginx additionally routes `http://<host>/admin` to the admin UI (via
`deploy/overrides/nginx/admin.conf`) and `/api/v1`, `/internal` and the mail
protocols to the backend. To run locally, copy `deploy/mailez.env.example` to
`deploy/mailez.env` (gitignored) and adjust `SECRET_KEY`, `DOMAIN` and
`HOSTNAMES` before `docker compose up`. TLS is off by default
(`TLS_FLAVOR=notls`); for local testing with TLS run
`deploy/scripts/generate-certs.ps1` (or `.sh`) and set `TLS_FLAVOR=cert` — see
`deploy/certs/README.md` for production guidance.

After the stack is up, run `go run ./cmd/e2e` from `backend/` (see
`deploy/scripts/README.md`) to verify the full mail path: SMTP submission,
IMAP delivery, DKIM signing and rspamd filtering.
