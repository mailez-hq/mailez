# mailess

Self-hosted mail server control plane, rewritten in Go + Next.js.

A parallel project inspired by [Mailu](https://github.com/Mailu/Mailu) (MIT): same
third-party components (nginx / Postfix / Dovecot / Rspamd), self-developed
control plane (admin, SSO, webmail) rebuilt with modern stack.

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
- Internal API consumed by the mail stack via Mailu's podop bridge: postfix maps,
  dovecot passdb/userdb/quota/sieve, nginx auth, client autoconfig, SRS, rate limit
- Admin UI (domains / users / aliases / relays / fetches / tokens / audit / config)
- Self-built webmail: IMAP/SMTP gateway with temp-token auth, three-pane UI,
  optional AI provider layer (summarize / draft)

The internal API wire contract is pinned by contract tests in
`backend/internal/internalapi` so upgrades of the Mailu mail images cannot break
the integration silently.

Deployment: `deploy/mailu.env` + `deploy/docker-compose.yml` wire the Mailu mail
images (nginx/postfix/dovecot/rspamd) to the mailess backend internal API at
`http://backend:8080` (the port the Mailu images hardcode). The backend listens
on 8080 in-container, published as `127.0.0.1:8081`. The admin and webmail
Next.js apps run as their own services, proxying `/api/v1` to the backend via
`API_TARGET=http://backend:8080`, and are published as `127.0.0.1:8082`
(admin, basePath `/admin`) and `127.0.0.1:8083` (webmail). The Mailu front
nginx additionally routes `http://<host>/admin` to the admin UI (via
`deploy/overrides/nginx/admin.conf`) and `/api/v1`, `/internal` and the mail
protocols to the backend. To run locally, copy `deploy/mailu.env.example` to
`deploy/mailu.env` (gitignored) and adjust `SECRET_KEY`, `DOMAIN` and
`HOSTNAMES` before `docker compose up`. TLS is off by default
(`TLS_FLAVOR=notls`); for local testing with TLS run
`deploy/scripts/generate-certs.ps1` (or `.sh`) and set `TLS_FLAVOR=cert` — see
`deploy/certs/README.md` for production guidance.
