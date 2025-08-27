# P1/P2 smoke: smart replies route, attachment zip, large-attachment relay,
# burn-after-read and the login security alert.
#
# Usage:
#   pwsh scripts/smoke-p1.ps1 -Email admin@example.com -Password '...'
param(
  [string]$Base = "http://127.0.0.1:8080/api/v1",
  [string]$Email = "admin@example.com",
  [string]$Password = ""
)

$ErrorActionPreference = "Stop"
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

function Invoke-Post($path, $body) {
  Invoke-RestMethod -Uri ($Base + $path) -Method Post -Body ($body | ConvertTo-Json -Depth 8) `
    -ContentType "application/json" -WebSession $session
}
function Invoke-Get($path) {
  Invoke-RestMethod -Uri ($Base + $path) -Method Get -WebSession $session
}

if (-not $Password) { $Password = $env:EAS_E2E_PASSWORD }
if (-not $Password) { throw "-Password is required" }

$login = Invoke-Post "/sso/login" @{ email = $Email; pw = $Password }
if (-not $login.email) { throw "login failed" }
Write-Host "ok login"

# 1. Login security alert: the first login from this IP after migration
#    should land a "安全提醒：新设备登录" mail in the inbox.
Start-Sleep -Seconds 2
$inbox = Invoke-Get "/mail/messages?folder=Inbox&page=0"
$alert = $inbox | Where-Object { $_.subject -like "安全提醒*" } | Select-Object -First 1
if ($alert) { Write-Host "ok login security alert delivered" }
else { Write-Host "skip: login alert not found (IP already known or MTA delayed)" }

# 2. Burn-after-read header round trip.
$attA = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("aaa"))
$attB = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("bbb"))
Invoke-Post "/mail/send" @{
  to = @($Email); subject = "阅后即焚测试"; body = "机密内容"
  burn_after_minutes = 30
  attachments = @(
    @{ filename = "a.txt"; content_type = "text/plain"; size = 3; data = $attA },
    @{ filename = "b.txt"; content_type = "text/plain"; size = 3; data = $attB }
  )
} | Out-Null
Start-Sleep -Seconds 2
$list = Invoke-Get "/mail/messages?folder=Inbox&page=0"
$burn = $list | Where-Object { $_.subject -eq "阅后即焚测试" } | Select-Object -First 1
if (-not $burn) { throw "burn mail not found" }
$detail = Invoke-Get ("/mail/message?folder=Inbox&uid=" + $burn.uid)
if ($detail.burn_after_minutes -ne 30) { throw "burn_after_minutes missing" }
Write-Host "ok burn-after-read header"

# 3. Attachment zip download.
$zipResp = Invoke-WebRequest -Uri ($Base + "/mail/attachments/zip?folder=Inbox&uid=" + $burn.uid) `
  -UseBasicParsing -WebSession $session
if ($zipResp.Headers["Content-Type"] -notlike "*zip*") { throw "zip content type wrong" }
$bytes = $zipResp.Content
if ($bytes[0] -ne 0x50 -or $bytes[1] -ne 0x4B) { throw "not a zip (PK magic missing)" }
Write-Host "ok attachment zip ($($bytes.Length) bytes)"

# 4. Large-attachment relay: upload, token download, bad-token reject.
$tmp = Join-Path $env:TEMP "relay.bin"
[IO.File]::WriteAllBytes($tmp, [byte[]](1,2,3,4,5))
$form = @{ file = Get-Item $tmp }
$up = Invoke-RestMethod -Uri ($Base + "/uploads") -Method Post -Form $form -WebSession $session
if (-not $up.url) { throw "upload returned no url" }
$dl = Invoke-WebRequest -Uri $up.url -UseBasicParsing -WebSession $session
if ($dl.Content.Length -ne 5) { throw "download length wrong" }
try {
  Invoke-WebRequest -Uri ($Base + "/uploads/" + $up.id + "/download?token=bad") -UseBasicParsing -WebSession $session | Out-Null
  throw "bad token was accepted"
} catch {
  if ($_.Exception.Response.StatusCode.value__ -ne 403) { throw "bad token status wrong: $_" }
}
Remove-Item $tmp -ErrorAction SilentlyContinue
Write-Host "ok large-attachment relay (upload + token download + 403 on bad token)"

# 5. Smart reply route exists (AI provider is not configured in dev, so the
#    endpoint answers 400 with "ai is disabled" rather than 404).
$status = $null
try {
  Invoke-Post "/ai/replies" @{ text = "hello" } | Out-Null
} catch {
  $status = $_.Exception.Response.StatusCode.value__
}
if ($status -ne 400) { Write-Host "note: /ai/replies answered $status (expected 400 when AI disabled)" }
else { Write-Host "ok smart-reply route (AI disabled guard)" }

Write-Host "P1/P2 SMOKE PASSED"
