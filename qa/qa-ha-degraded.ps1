# qa-ha-degraded.ps1 - multi-active edge cases the first HA pass did not cover:
#   1) BOTH engine nodes down: does the API fail fast with a clear error (not hang)?
#   2) long-lived SSE + IMAP IDLE behaviour while one node is stopped and returns
#   3) recovery: no loss / no duplicate after the outage
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== HA degraded-mode probe rnd=$rnd =="

function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}
function ImapSubjects([string]$email, [string]$pw, [string]$folder = "INBOX") {
    $out = @(& python .\qa-imap-op.py 127.0.0.1 143 $email $pw subjects $folder 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return @() }
    $obj = $json | ConvertFrom-Json
    if (-not $obj.subjects) { return @() }
    return @($obj.subjects | ForEach-Object { $_[1] })
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-ha2-a-$rnd@example.com"
$B = "qa-ha2-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA HA2" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

# open an SSE stream so we can watch IDLE/SSE behaviour through the outage
$cookieFile = "D:\code\mailez-hq\mailez\qa\.ha2-cookie-$rnd.txt"
$sseLog = "D:\code\mailez-hq\mailez\qa\.ha2-sse-$rnd.log"
$cookies = $bSess.S.Cookies.GetCookies($Base)
Set-Content -LiteralPath $cookieFile -Value (($cookies | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join "; ") -Encoding UTF8
$procArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ".\qa-sse-listen.ps1",
    "-Url", "$Base/events", "-CookieFile", $cookieFile, "-Out", $sseLog, "-Seconds", "420")
$sse = Start-Process pwsh -ArgumentList $procArgs -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 6
$ready = (Select-String -LiteralPath $sseLog -Pattern "event: ready" -Encoding UTF8 -ErrorAction SilentlyContinue | Select-Object -First 1) -ne $null
"sse ready=$ready"

# --- baseline delivery
$s1 = "ha2-before-$rnd"
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = $s1; body = "before outage" })
Start-Sleep -Seconds 6
$seenBefore = @(ImapSubjects $B $PW | Where-Object { $_ -match [regex]::Escape($s1) }).Count
Check HA2.1 "baseline delivery before the outage" ($seenBefore -ge 1) "copies=$seenBefore"

# --- stop BOTH engine nodes
$null = docker stop mailez-mailezine-1 2>&1
$null = docker stop mailez-mailezine-2 2>&1
Start-Sleep -Seconds 8
$nodes = docker ps --format "{{.Names}}" | Select-String "mailez-mailezine"
"running engine nodes now: [$($nodes -join ',')]"

$sw = [Diagnostics.Stopwatch]::StartNew()
$s2 = "ha2-during-$rnd"
$send2 = Api $aSess POST "/mail/send" @{ to = @($B); subject = $s2; body = "during outage" }
$sendMs = [int]$sw.Elapsed.TotalMilliseconds
$body2 = ""
if ($send2.Raw) { $body2 = [Text.Encoding]::UTF8.GetString($send2.Raw.RawContentStream.ToArray()) }
Check HA2.2 "with both engines down the send fails fast and clearly (no hang)" `
    ($send2.Status -ge 400 -and $sendMs -lt 60000) `
    "status=$($send2.Status) latencyMs=$sendMs body=$($body2.Substring(0, [Math]::Min(140, $body2.Length)))"

$sw2 = [Diagnostics.Stopwatch]::StartNew()
$listDown = Api $bSess GET "/mail/messages?folder=Inbox&page=0"
$listMs = [int]$sw2.Elapsed.TotalMilliseconds
$body3 = ""
if ($listDown.Raw) { $body3 = [Text.Encoding]::UTF8.GetString($listDown.Raw.RawContentStream.ToArray()) }
Check HA2.3 "with both engines down the mailbox read fails fast and clearly" `
    ($listDown.Status -ge 400 -and $listMs -lt 60000) `
    "status=$($listDown.Status) latencyMs=$listMs body=$($body3.Substring(0, [Math]::Min(140, $body3.Length)))"

# the webmail API host itself must stay healthy
$health = Api $admin GET "/health"
Check HA2.4 "control plane stays healthy while the engines are down" ($health.Status -eq 200) "health=$($health.Status)"

# --- bring them back
$null = docker start mailez-mailezine-1 2>&1
$null = docker start mailez-mailezine-2 2>&1
Start-Sleep -Seconds 35
$nodesUp = docker ps --format "{{.Names}}" | Select-String "mailez-mailezine"
"engine nodes back: [$($nodesUp -join ',')]"

$s3 = "ha2-after-$rnd"
$send3 = Api $aSess POST "/mail/send" @{ to = @($B); subject = $s3; body = "after outage" }
$seenAfter = 0
$sw3 = [Diagnostics.Stopwatch]::StartNew()
while ($sw3.Elapsed.TotalSeconds -lt 150 -and $seenAfter -lt 1) {
    $seenAfter = @(ImapSubjects $B $PW | Where-Object { $_ -match [regex]::Escape($s3) }).Count
    if ($seenAfter -lt 1) { Start-Sleep -Seconds 4 }
}
Check HA2.5 "delivery resumes after both engines return" ($send3.Status -in @(200, 204) -and $seenAfter -ge 1) `
    "send=$($send3.Status) copies=$seenAfter in=$([int]$sw3.Elapsed.TotalSeconds)s"

# a message that could not be accepted during the outage must simply not exist
$ghost = @(ImapSubjects $B $PW | Where-Object { $_ -match [regex]::Escape($s2) }).Count
$finalBefore = @(ImapSubjects $B $PW | Where-Object { $_ -match [regex]::Escape($s1) }).Count
Check HA2.6 "no partial/duplicate mail from the outage window" ($ghost -eq 0 -and $finalBefore -eq 1) `
    "duringOutageCopies=$ghost baselineCopies=$finalBefore"

# --- SSE behaviour: the stream should have survived or re-established
$sseText = if (Test-Path $sseLog) { Get-Content -LiteralPath $sseLog -Raw -Encoding UTF8 } else { "" }
$pings = ([regex]::Matches($sseText, ": ping")).Count
$eof = $sseText -match "#eof"
$err = $sseText -match "#error"
$frames = ([regex]::Matches($sseText, "event: mail")).Count
Check HA2.7 "SSE stream survives an engine outage (or reports it) without dying silently" `
    (-not $err -and ($pings -ge 3 -or $frames -ge 1)) `
    "pings=$pings mailFrames=$frames eof=$eof streamError=$err"

if ($sse -and -not $sse.HasExited) { Stop-Process -Id $sse.Id -Force -ErrorAction SilentlyContinue }
Remove-Item -LiteralPath $cookieFile -Force -ErrorAction SilentlyContinue
$cl = @()
foreach ($u in @($A, $B)) { $cl += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cl -join ' ')"
Flush-Results "HA2"
"== done =="
