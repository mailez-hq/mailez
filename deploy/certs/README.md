# TLS certificates

The mailez `gateway` (nginx) container reads TLS material from `./certs` (mounted
at `/certs`). Two files are expected:

- `cert.pem` — the server certificate (fullchain for production)
- `key.pem`  — the matching private key

## Development

Generate a self-signed pair for local testing:

```sh
./scripts/generate-certs.sh mail.example.com
```

or on Windows:

```powershell
.\scripts\generate-certs.ps1 -Hostname mail.example.com
```

Then switch `TLS_FLAVOR=cert` in `deploy/mailez.env` and restart `gateway`.

## Production

Do **not** use self-signed certificates for real mail servers — mail clients
and peer MTAs will reject them. Options:

- Place a real certificate (or a provider fullchain) as `cert.pem` + `key.pem`,
- or use `TLS_FLAVOR=letsencrypt` / `mail-letsencrypt` with ports 80/443
  reachable for ACME challenges.

Certificate files are gitignored on purpose.
