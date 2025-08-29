# Local dev backend for mailez. Defaults to the ENTERPRISE build (full
# feature set: LDAP, DLP, archive, AI, S/MIME, sharing...). Pass -Ce to
# run the community build instead, where enterprise features show their
# "requires the enterprise edition" degradation.
param([switch]$Ce)

$env:TMP = "$env:SystemRoot\TEMP"
$env:TEMP = "$env:SystemRoot\TEMP"
$env:GOCACHE = "$env:LOCALAPPDATA\go-build"
$env:GOTMPDIR = "$env:SystemRoot\TEMP"
$env:MAILEZ_PORT = "8080"
$env:MAILEZ_ENV = "development"
$env:MAILEZ_SECRET_KEY = "mailez-dev-secret-key-2026"
$env:DB_DSN = "$PSScriptRoot\mailez.db"
$env:REDIS_ADDR = "127.0.0.1:17379"
$env:MAIL_IMAP_ADDR = "127.0.0.1:143"
$env:MAIL_SMTP_ADDR = "127.0.0.1:1587"
$env:MAIL_SIEVE_ADDR = "127.0.0.1:4190"
$env:MAIL_MTA_ADDR = "127.0.0.1:25"
$env:MAILEZ_DOMAIN = "example.com"
$env:MAILEZ_HOSTNAME = "localhost"
$env:LOG_LEVEL = "info"
# AI provider is configured through the admin console (Configuration -> AI),
# stored encrypted in the database. No environment variables needed here.
if ($Ce) { go run ./cmd/server } else { go run -tags mailez_ee ./cmd/server }
