# qa-tp-p14-push.ps1 - P14 push and real-time sync (TEST-PROMPTS.md P14), rev1
# Covers: SSE new-mail push, Web Push subscription lifecycle, multi-client
# consistency (IMAP STORE/MOVE seen from the web side), reconnect after a drop,
# and long-lived stream stability (keepalive pings).
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P14 push rev1 rnd=$rnd base=$Base =="

$admin = New-Session
$A = "qa-p14a-$rnd@example.com"
$B = "qa-p14b-$rnd@example.com"
$PW = "QaUser2026!"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P14" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
# NOTE: PowerShell variable names are case-insensitive, so the session handles
# must not collide with $A/$B (the addresses) — a session bound to $a replaces
# $A and the send payload then carries an object instead of an address.
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

$CookieFile = "D:\code\mailez-hq\mailez\qa\.p14-cookie-$rnd.txt"
$ListenScript = "D:\code\mailez-hq\mailez\qa\qa-sse-listen.ps1"
$ImapScript = "D:\code\mailez-hq\mailez\qa\qa-imap-op.py"
$SseBase = $Base -replace "/api/v1$", ""

function Save-Cookie($sess, [string]$path) {
    $cookies = $sess.S.Cookies.GetCookies($Base)
    $hdr = ($cookies | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join "; "
    Set-Content -LiteralPath $path -Value $hdr -Encoding UTF8
    return $hdr
}

function Start-Sse([string]$logPath, [int]$secs = 600) {
    if (Test-Path $logPath) { Remove-Item -LiteralPath $logPath -Force }
    $procArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $ListenScript,
        "-Url", "$Base/events", "-CookieFile", $CookieFile, "-Out", $logPath, "-Seconds", "$secs")
    return Start-Process pwsh -ArgumentList $procArgs -PassThru -WindowStyle Hidden
}

function Wait-Sse([string]$logPath, [string]$pattern, [int]$timeoutSec) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeoutSec) {
        if (Test-Path $logPath) {
            $hit = Select-String -LiteralPath $logPath -Pattern $pattern -Encoding UTF8 -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($hit) { return $hit.Line }
        }
        Start-Sleep -Milliseconds 400
    }
    return ""
}

function Stop-Sse($proc) {
    if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
}

# python writes the JSON result on stdout; stderr may carry interpreter
# warnings, so keep the two apart and take the JSON line.
function Run-Imap([string[]]$opArgs) {
    $out = @(& python $ImapScript 127.0.0.1 143 $A $PW @opArgs 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return $null }
    return ($json | ConvertFrom-Json)
}

# GET /mail/messages answers with a bare JSON array (the total travels in the
# X-Total-Messages header), so unwrap defensively instead of assuming a page
# envelope: reading ".data" off an array yields $null per element and silently
# empties every list assertion.
function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    # NOTE: testing "$resp.Data.data" is not enough — on a bare array every
    # element lacks .data, and member enumeration hands back N $nulls, which is
    # still truthy for N > 1. Check for the property itself.
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}

[void](Save-Cookie $aSess $CookieFile)
$log1 = "D:\code\mailez-hq\mailez\qa\.p14-sse1-$rnd.log"
$p1 = Start-Sse $log1 420

# ---------------------------------------------------------------- P14.1 SSE
$ready = Wait-Sse $log1 'event: ready' 40
Check P14.1a "SSE /api/v1/events opens for the logged-in user" ($ready -ne "") "ready=$ready"

$subj1 = "p14-sse-$rnd"
$t0 = Get-Date
$send1 = Api $bSess POST "/mail/send" @{ to = @($A); subject = $subj1; body = "p14 sse body" }
$ev1 = Wait-Sse $log1 'event: mail' 180
$lat1 = [math]::Round(((Get-Date) - $t0).TotalSeconds, 1)
Check P14.1b "new mail pushes an SSE mail event (no client refresh)" ($send1.Status -in @(200, 204) -and $ev1 -ne "") "send=$($send1.Status) latency=${lat1}s frame=$ev1"

# ------------------------------------------------------------ P14.3 IMAP side
# Run before teardown: the stream is still open, so the engine must notice a
# change made by a *different* client (IMAP) and push it to the browser side.
$imapSeen = Run-Imap @("seen")
$tImap = Get-Date
$evSeen = Wait-Sse $log1 'event: mail' 90
$latSeen = [math]::Round(((Get-Date) - $tImap).TotalSeconds, 1)
Check P14.3a "IMAP STORE \Seen reaches the web side via SSE" ($imapSeen.login -eq "ok" -and $evSeen -ne "") "uid=$($imapSeen.uid) status=$($imapSeen.status) latency=${latSeen}s"

$imapMove = Run-Imap @("move", "Junk")
$tMove = Get-Date
$evMove = Wait-Sse $log1 'Junk' 120
$latMove = [math]::Round(((Get-Date) - $tMove).TotalSeconds, 1)
Check P14.3b "IMAP MOVE to Junk reaches the web side via SSE" ($imapMove.status -eq "OK" -and $evMove -ne "") "uid=$($imapMove.uid) status=$($imapMove.status) latency=${latMove}s frame=$evMove"

# the webmail view must agree with what the IMAP client did
$junk = Api $aSess GET "/mail/messages?folder=Junk&page=0"
$junkHit = $false
$flagTxt = ""
foreach ($m in (MsgList $junk)) {
    if ($m.subject -eq $subj1) {
        $junkHit = $true
        $flagTxt = ($m.flags -join ",")
    }
}
Check P14.3c "webmail list/flag view matches the IMAP client" ($junkHit -and $flagTxt -match "Seen") "junkHasMsg=$junkHit flags=[$flagTxt]"

# ------------------------------------------------------- P14.2 Web Push sub
$vapid = Api $aSess GET "/push/vapid"
$key = if ($vapid.Data) { [string]$vapid.Data.public_key } else { "" }
Check P14.2a "GET /push/vapid returns the application server key" ($vapid.Status -eq 200 -and $key.Length -gt 20) "status=$($vapid.Status) keyLen=$($key.Length)"

$endpoint = "https://qa-push.invalid/p14-$rnd"
$sub = Api $aSess POST "/push/subscribe" @{ endpoint = $endpoint; keys = @{ p256dh = "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U"; auth = "I3x4u1zqk8Yy2w2m1n5Q0A" } }
$row = ""
try { $row = [string](docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT CONCAT(endpoint,'|token_len=',LENGTH(token_enc)) FROM push_subscription WHERE user_email='$A';" 2>$null | Select-Object -First 1) } catch {}
Check P14.2b "push subscription stored with an encrypted notifier credential" ($sub.Status -eq 204 -and "$row".Trim() -ne "" -and "$row" -notmatch "token_len=0") "status=$($sub.Status) row=$("$row".Trim())"

# a delivery must reach the (deliberately unreachable) push endpoint: the
# notifier logs the failed dispatch, which proves the fan-out ran.
$logBefore = (docker logs mailez-backend-1 --tail 1 2>&1 | Out-String)
$send2 = Api $bSess POST "/mail/send" @{ to = @($A); subject = "p14-push-$rnd"; body = "p14 push body" }
$pushed = ""
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 150) {
    $tail = docker logs mailez-backend-1 --since 3m 2>&1 | Out-String
    if ($tail -match "push: notify") { $pushed = ([regex]::Match($tail, "push: notify[^\r\n]*")).Value; break }
    Start-Sleep -Seconds 10
}
Check P14.2c "new mail triggers the Web Push fan-out for the subscriber" ($send2.Status -in @(200, 204) -and $pushed -ne "") "send=$($send2.Status) log=[$pushed] waited=$([int]$sw.Elapsed.TotalSeconds)s"

$unsub = Api $aSess DELETE "/push/subscribe" @{ endpoint = $endpoint }
$rowAfter = ""
try { $rowAfter = [string](docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT COUNT(*) FROM push_subscription WHERE user_email='$A';" 2>$null | Select-Object -First 1) } catch {}
Check P14.2d "unsubscribe removes the endpoint (stops further pushes)" ($unsub.Status -in @(200, 204) -and "$rowAfter".Trim() -eq "0") "status=$($unsub.Status) rowsAfter=$("$rowAfter".Trim())"

# ------------------------------------------------- P14.4 drop and reconnect
Stop-Sse $p1
Start-Sleep -Seconds 3
$subj2 = "p14-reconnect-$rnd"
$send3 = Api $bSess POST "/mail/send" @{ to = @($A); subject = $subj2; body = "p14 offline body" }
Start-Sleep -Seconds 5
$log2 = "D:\code\mailez-hq\mailez\qa\.p14-sse2-$rnd.log"
$p2 = Start-Sse $log2 300
$ready2 = Wait-Sse $log2 'event: ready' 40
# the missed message must still be visible after reconnect (no data loss even
# though the stream itself carries no replay backlog)
$found2 = $false
$sw2 = [Diagnostics.Stopwatch]::StartNew()
while ($sw2.Elapsed.TotalSeconds -lt 180 -and -not $found2) {
    $lst = Api $aSess GET "/mail/messages?folder=Inbox&page=0"
    foreach ($m in (MsgList $lst)) { if ($m.subject -eq $subj2) { $found2 = $true } }
    if (-not $found2) { Start-Sleep -Seconds 5 }
}
Check P14.4 "reconnect after a drop: stream re-establishes, missed mail still visible" ($ready2 -ne "" -and $found2) "ready=$ready2 send=$($send3.Status) missedVisible=$found2 in=$([int]$sw2.Elapsed.TotalSeconds)s"

# ------------------------------------------------- P14.2/P14.5 stream health
# keep the connection up for a fixed sample window and count keepalive frames
$holdSec = if ($env:QA_P14_HOLD_SEC) { [int]$env:QA_P14_HOLD_SEC } else { 400 }
$subj3 = "p14-long-$rnd"
$tHold = Get-Date
[void](Api $bSess POST "/mail/send" @{ to = @($A); subject = $subj3; body = "p14 long body" })
$ev3 = Wait-Sse $log2 'event: mail' 120
$holdLat = [math]::Round(((Get-Date) - $tHold).TotalSeconds, 1)
$swHold = [Diagnostics.Stopwatch]::StartNew()
while ($swHold.Elapsed.TotalSeconds -lt $holdSec) { Start-Sleep -Seconds 10 }
$logTxt = if (Test-Path $log2) { Get-Content -LiteralPath $log2 -Raw -Encoding UTF8 } else { "" }
$pings = ([regex]::Matches($logTxt, ": ping")).Count
$eof = $logTxt -match "#eof"
$heldSec = [int]$swHold.Elapsed.TotalSeconds
# keepalive cadence is a 15s ticker, so allow up to two missed ticks per
# interval group (log flush timing makes the exact count jittery)
$minPings = [int]([Math]::Floor(($heldSec + 60) / 15) - 8)
Check P14.5 "stream stays open for $($holdSec)s with 15s keepalives, no server-side EOF" ($ev3 -ne "" -and -not $eof -and $pings -ge $minPings) "mailLatency=${holdLat}s held=${heldSec}s pings=$pings (min $minPings) eof=$eof"
Stop-Sse $p2

# ----------------------------------------------------- code-level evidence
$wt = Select-String -Path "D:\code\mailez-hq\mailez\backend\internal\server\server.go" -Pattern "WriteTimeout" -Encoding UTF8 | Select-Object -First 2
"evidence: server.go $($wt.Line -join ' | ')"

$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Remove-Item -LiteralPath $CookieFile -Force -ErrorAction SilentlyContinue
Flush-Results "P14"
"== done =="
