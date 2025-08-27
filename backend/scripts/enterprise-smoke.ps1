# Enterprise feature smoke test against a running mailez backend:
# read receipts, mail recall, mail merge, mark-all-read and calendar
# sharing/reminders/subscription feed.
#
# Usage:
#   pwsh scripts/enterprise-smoke.ps1 -Email admin@example.com -Password '...'
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

if (-not $Password) {
  $Password = $env:EAS_E2E_PASSWORD
}
if (-not $Password) { throw "-Password is required" }

# 1. Login
$login = Invoke-Post "/sso/login" @{ email = $Email; pw = $Password }
if (-not $login.email) { throw "login failed" }
Write-Host "ok login"

# 2. Send a mail requesting a read receipt (to self)
Invoke-Post "/mail/send" @{ to = @($Email); subject = "回执测试"; body = "请回执"; receipt_requested = $true } | Out-Null
Start-Sleep -Seconds 2
$list = Invoke-Get "/mail/messages?folder=Inbox&page=0"
$found = $list | Where-Object { $_.subject -eq "回执测试" } | Select-Object -First 1
if (-not $found) {
  $sent = Invoke-Get "/mail/messages?folder=Sent&page=0"
  $found = $sent | Where-Object { $_.subject -eq "回执测试" } | Select-Object -First 1
}
if (-not $found) { throw "receipt mail not found" }
$folder = if ($found.folder) { $found.folder } else { "Inbox" }
$detail = Invoke-Get ("/mail/message?folder=" + [uri]::EscapeDataString($folder) + "&uid=" + $found.uid)
if (-not $detail.receipt_requested) { throw "receipt_requested missing" }
Write-Host "ok read-receipt request detected"

# 3. Send the receipt; the message must gain the \$MDNSent keyword
Invoke-Post "/mail/receipt" @{ folder = $folder; uid = $found.uid } | Out-Null
Start-Sleep -Seconds 1
$detail2 = Invoke-Get ("/mail/message?folder=" + [uri]::EscapeDataString($folder) + "&uid=" + $found.uid)
if ($detail2.flags -notcontains '$MDNSent') { throw "MDNSent flag missing" }
Write-Host "ok receipt sent"

# 4. Mail merge (逐封群发) with {{name}}/{{code}} substitution
$merge = Invoke-Post "/mail/merge" @{
  subject = "群发 {{name}} 工号{{code}}"
  body    = "您好 {{name}}"
  recipients = @(@{ email = $Email; name = "管理员"; vars = @{ code = "A100" } })
}
if ($merge.sent -ne 1) { throw "merge sent $($merge.sent)" }
Start-Sleep -Seconds 2
$list2 = Invoke-Get "/mail/messages?folder=Inbox&page=0"
$mergeMsg = $list2 | Where-Object { $_.subject -like "群发*" } | Select-Object -First 1
if (-not $mergeMsg) {
  $sent2 = Invoke-Get "/mail/messages?folder=Sent&page=0"
  $mergeMsg = $sent2 | Where-Object { $_.subject -like "群发*" } | Select-Object -First 1
}
if (-not $mergeMsg) { throw "merge mail not found" }
if ($mergeMsg.subject -ne "群发 管理员 工号A100") { throw "merge substitution wrong: $($mergeMsg.subject)" }
Write-Host "ok mail merge + substitution"

# 5. Recall the merged mail from Sent
$sentList = Invoke-Get "/mail/messages?folder=Sent&page=0"
$recallTarget = $sentList | Where-Object { $_.subject -eq "群发 管理员 工号A100" } | Select-Object -First 1
if ($recallTarget) {
  $rc = Invoke-Post "/mail/recall" @{ folder = "Sent"; uid = $recallTarget.uid }
  Start-Sleep -Seconds 2
  $recallDetail = Invoke-Get ("/mail/message?folder=Sent&uid=" + $recallTarget.uid)
  if ($recallDetail.flags -notcontains '$RecallSent') { throw "RecallSent flag missing" }
  Write-Host "ok mail recall (notified=$($rc.notified))"
} else {
  Write-Host "skip recall: merged mail not visible in Sent"
}

# 6. Mark the whole folder as read
Invoke-Post "/mail/read-all" @{ folder = "Inbox" } | Out-Null
Write-Host "ok mark all read"

# 7. Calendar: create event with reminder, verify feed/export/shares
$start = (Get-Date).AddHours(1).ToString("o")
$ev = Invoke-Post "/calendar/events" @{ summary = "E2E 会议"; start = $start; reminder_minutes = 5 }
if ($ev.reminder_minutes -ne 5) { throw "reminder not saved" }
$feed = Invoke-Get "/calendar/feed"
$export = Invoke-WebRequest -Uri $feed.url -UseBasicParsing -WebSession $session
if ($export.Content -notlike "*E2E 会议*") { throw "export missing event" }
$shares = Invoke-Get "/calendar/shares"
Write-Host "ok calendar event+reminder+feed+export (shares owned=$($shares.owned.Count))"

Write-Host "E2E PASSED"
