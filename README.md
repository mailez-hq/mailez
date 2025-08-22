# mailess

Self-hosted mail server control plane, rewritten in Go + Next.js.

A parallel project inspired by [mainstream suite](https://github.com/mainstream suite/mainstream suite) (MIT): same
third-party components (nginx / legacy MTA / legacy IMAP / Rspamd), self-developed
control plane (admin, SSO, webmail) rebuilt with modern stack.

## Stack

- Backend: Go + Fiber, GORM (SQLite dev / MySQL prod), Redis
- Frontend: Next.js App Router + React, shadcn/ui
- Webmail: self-built IMAP client (go-imap) with optional AI plugin layer
- License: Apache 2.0

## Layout

```
backend/    Go control plane (admin API, internal API, SSO, configgen, podop, mail gateway, ai)
frontend/   Next.js monorepo (apps/admin, apps/webmail, packages/ui)
deploy/     Docker Compose (nginx/legacy MTA/IMAP stack/rspamd + backend/frontend)
```

## Status

Phase 0 scaffolding. See docs/plan for the full roadmap.
