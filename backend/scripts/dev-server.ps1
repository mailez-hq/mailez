#!/usr/bin/env pwsh
# Starts the mailez backend in dev mode (host-run, control plane on :8080).
# Prerequisite: the mail services are up, e.g.
#   cd deploy; docker compose -f docker-compose.dev.yml up -d
#
# DOVECOT_ADDRESS / POSTFIX_ADDRESS must be the *fixed container IPs* declared in
# docker-compose.dev.yml: the internal API returns them as Auth-Server and
# nginx's mail auth module only accepts IP literals. MAIL_*_ADDR are the
# host-mapped proxy ports the backend itself uses to reach the mail services.
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..\..')

$env:GOCACHE = Join-Path $root 'backend\.gocache-dev'
$env:MAILEZ_PORT = '8080'
$env:DB_DSN = Join-Path $root 'backend\mailez.db'
$env:MAIL_IMAP_ADDR = '127.0.0.1:1143'
$env:MAIL_SMTP_ADDR = '127.0.0.1:1587'
$env:MAIL_SIEVE_ADDR = '127.0.0.1:4190'
$env:DOVECOT_ADDRESS = '192.168.206.5'
$env:POSTFIX_ADDRESS = '192.168.206.4'

Set-Location (Join-Path $root 'backend')
go run ./cmd/server
