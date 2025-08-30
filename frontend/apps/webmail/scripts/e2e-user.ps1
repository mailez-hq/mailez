# Creates the dedicated webmail e2e account (see e2e/README.md).
# Requires an admin session on a running mailez stack; safe to re-run
# (creating an existing user just fails harmlessly).
param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPw = "MailezDemo2026!",
    [string]$E2eUser = "e2esmoke",
    [string]$E2eDomain = "example.com",
    [string]$E2ePassword = "E2eSmoke2026!"
)

$ErrorActionPreference = "Stop"

$session = "$env:TEMP\mailez-e2e-admin-cookies.txt"
Remove-Item $session -ErrorAction SilentlyContinue

# 1. Log in as admin (SSO session cookie lands in the jar).
$login = Invoke-WebRequest -Uri "$BaseUrl/api/v1/sso/login" -Method Post `
    -ContentType "application/json" -SessionVariable web `
    -Body (@{ email = $AdminEmail; pw = $AdminPw } | ConvertTo-Json)
if ($login.StatusCode -ne 200) { throw "admin login failed: $($login.StatusCode)" }

# 2. Create (or confirm) the throwaway e2e user.
$email = "$E2eUser@$E2eDomain"
try {
    Invoke-WebRequest -Uri "$BaseUrl/api/v1/users" -Method Post `
        -WebSession $web -ContentType "application/json" `
        -Body (@{
            email         = $email
            password      = $E2ePassword
            displayed_name = "E2E Smoke"
            enabled       = $true
        } | ConvertTo-Json) | Out-Null
    Write-Host "e2e-user: created $email"
} catch [Microsoft.PowerShell.Commands.HttpResponseException] {
    # A duplicate user is the expected outcome on re-runs.
    Write-Host "e2e-user: $email already exists ($($_.Exception.Response.StatusCode))"
}
Write-Host "e2e-user: ready — run 'npm run e2e' in frontend/apps/webmail"
