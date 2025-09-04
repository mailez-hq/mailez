# Contributing

## Development setup

Follow [`docs/dev-setup.md`](docs/dev-setup.md) to bring up the mail services and
the backend/frontends in development mode. The short version:

```sh
cd deploy && cp mailez.env.example mailez.env   # set SECRET_KEY
docker compose -f docker-compose.dev.yml up -d
cd backend && powershell -File .\dev-start.ps1    # API on :8080
cd frontend/apps/webmail && npm run dev -- -p 3001
cd frontend/apps/admin && npm run dev -- -p 3000
```

## Architecture

See [`docs/architecture.md`](docs/architecture.md). The backend is organized
into domain packages (`user`, `domain`, `alias`, `mailbox`, `compose`,
`contacts`, `sieve`, `admin`, `fetch`, `push`, `ai`) wired by the
`server` composition root; the frontend mirrors those domains under
`components/` and shares wire types through `frontend/packages/types`.

## Before submitting

- `make backend-verify` — gofmt, vet, `go mod tidy -diff`, tests
- `make frontend-verify` — typecheck, lint, tests for both apps
- `make e2e` — end-to-end smoke against the dev stack

CI runs exactly these checks, so `make verify` must be green locally.

## Commit conventions

- Imperative subject, ≤ 72 chars: `fix:`, `feat:`, `refactor:`, `docs:`,
  `chore:`, `test:`
- One logical change per commit; keep working trees clean
- Never commit secrets, `deploy/*.env`, `*.db`, or local logs (see `.gitignore`)

## License

By contributing you agree your work is licensed under the project license
described in `LICENSE`.
