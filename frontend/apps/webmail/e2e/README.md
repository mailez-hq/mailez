# Webmail e2e (Playwright)

The specs run against a **fully deployed mailez stack**. Nothing is mocked:
mail sent by the specs goes through the real SMTP path and lands in a real
mailbox.

## The e2e account

The suite drives a **dedicated throwaway account** — `e2esmoke` on
`example.com` (password `E2eSmoke2026!`), NOT the seeded admin. Two reasons:

- isolation: runs never touch (or depend on) a real inbox;
- safety: a fresh user gets a fresh engine mailbox store, so specs can't be
  broken by pre-existing data and can't break anyone's data.

Create it once per stack (admin session required):

```sh
curl -sf -c /tmp/e2e-admin-cookies.txt -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","pw":"MailezDemo2026!"}' \
  http://localhost:8080/api/v1/sso/login
curl -sf -b /tmp/e2e-admin-cookies.txt -H 'Content-Type: application/json' \
  -d '{"email":"e2esmoke@example.com","password":"E2eSmoke2026!","displayed_name":"E2E Smoke","enabled":true}' \
  http://localhost:8080/api/v1/users
```

The CI workflow (`.github/workflows/e2e.yml`) does exactly this after
seeding. Override the credentials with `MAILWEB_E2E_USER` /
`MAILWEB_E2E_PASSWORD` / `MAILWEB_E2E_DOMAIN` if your stack differs.

> Login rate limit: the backend throttles login attempts per IP
> (30 / 15 min by default). Re-running the suite repeatedly can trip it —
> wait for the window or flush the `mailez:login:*` keys in redis.

## Bring up the stack

Full containerized stack (community edition — this is what CI runs):

```sh
cd backend
go run ./cmd/build-images        # one-time: builds mailez/*:local images

cd ../deploy
cp mailez.env.example mailez.env # set MAILEZ_SECRET_KEY / MAILEZ_DOMAIN
docker compose -f docker-compose.community.yml up -d --build

cd ../backend
DB_DRIVER=mysql \
DB_DSN='mailez:mailez@tcp(127.0.0.1:13306)/mailez?charset=utf8mb4&parseTime=True&loc=Local' \
  go run ./cmd/seed              # creates the admin account
```

webmail is then at `http://localhost:8083` (the default base URL).

Local dev stacks (docker-compose.dev.yml + host backend + `next dev`) work
too — point the suite at your dev server, e.g.
`MAILWEB_E2E_BASE_URL=http://localhost:3001 npm run e2e`.

## Run the specs

```sh
cd frontend/apps/webmail

npx playwright install chromium   # one-time
npm run e2e
```

Reports and traces land in `playwright-report/` and `test-results/`
(gitignored).

## What is covered

- `auth.spec.ts` — login page renders, wrong password rejected,
  login → mailbox → sign out.
- `mailbox.spec.ts` — the main chain: compose a self-addressed message →
  the compose dialog closes → the message appears in **Sent** → open it in
  the reading pane (body rendered) → keyword search finds it → clear search
  restores the list; plus folder navigation. Each run stamps a unique
  subject so specs stay independent of mailbox contents.

Two product behaviors the specs encode (worth knowing before "fixing" them):

- sending shows no toast — the compose dialog closing is the success signal;
- the search box executes on Enter (there is a "Search ↵" affordance), it
  does not search as you type.
