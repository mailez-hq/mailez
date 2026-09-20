# qa-tp-p14b-multiclient.ps1 - P14.3 re-test with a convergence window, rev1
#
# The first P14 run read the IMAP side a few seconds after the SSE fired and
# hit the multi-active visibility window (the backend/IMAP client reaches the
# engine through engine-lb, which round-robins between the two mailezine
# nodes). This re-test waits for the message to be visible to the IMAP client
# first, then drives the cross-client checks, and finally gives the webmail
# list the same window before asserting.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P14.3 multiclient rev2 rnd=$rnd base=$Base =="

$admin = New-Session
$A = "qa-p14c-a-$rnd@example.com"
$B = "qa-p14c-b-$rnd@example.com"
$PW = "QaUser2026!"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P14c" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

$CookieFile = "D:\code\mailez-hq\mailez\qa\.p14c-cookie-$rnd.txt"
$ListenScript = "D:\code\mailez-hq\mailez\qa\qa-sse-listen.ps1"
$ImapScript = "D:\code\mailez-hq\mailez\qa\qa-imap-op.py"

function Save-Cookie($sess, [string]$path) {
    $cookies = $sess.S.Cookies.GetCookies($Base)
    $hdr = ($cookies | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join "; "
    Set-Content -LiteralPath $path -Value $hdr -Encoding UTF8
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
# Same, but only matching frames written after $from — otherwise a frame from
# an earlier step would satisfy the assertion.
function Wait-SseFrom([string]$logPath, [int]$from, [string]$pattern, [int]$timeoutSec) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeoutSec) {
        if (Test-Path $logPath) {
            $lines = @(Get-Content -LiteralPath $logPath -Encoding UTF8 -ErrorAction SilentlyContinue)
            for ($i = $from; $i -lt $lines.Count; $i++) {
                if ($lines[$i] -match $pattern) { return $lines[$i] }
            }
        }
        Start-Sleep -Milliseconds 400
    }
    return ""
}
function Log-Lines([string]$logPath) {
    if (-not (Test-Path $logPath)) { return 0 }
    return @(Get-Content -LiteralPath $logPath -Encoding UTF8 -ErrorAction SilentlyContinue).Count
}
function Run-Imap([string[]]$opArgs) {
    $out = @(& python $ImapScript 127.0.0.1 143 $A $PW @opArgs 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return $null }
    return ($json | ConvertFrom-Json)
}
# /mail/messages returns a bare array (total lives in X-Total-Messages).
function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
# Wait until the IMAP client can see the seeded message in the given folder.
function Wait-ImapUid([string]$folder, [int]$timeoutSec) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeoutSec) {
        $res = Run-Imap @("listuid", $folder)
        if ($res -and $res.uids -and @($res.uids).Count -gt 0) {
            return @{ uid = (@($res.uids)[-1]); waited = [int]$sw.Elapsed.TotalSeconds }
        }
        Start-Sleep -Seconds 5
    }
    return @{ uid = ""; waited = [int]$sw.Elapsed.TotalSeconds }
}

Save-Cookie $aSess $CookieFile
$log = "D:\code\mailez-hq\mailez\qa\.p14c-sse-$rnd.log"
$proc = Start-Sse $log 600
$ready = Wait-Sse $log 'event: ready' 40
"stream ready=$($ready -ne '')"

$subj = "p14c-multi-$rnd"
$send = Api $bSess POST "/mail/send" @{ to = @($A); subject = $subj; body = "p14c multiclient body" }
"send=$($send.Status)"
$visible = Wait-ImapUid "INBOX" 180
$ev1 = Wait-Sse $log 'event: mail' 60
Check P14.3a-rev2 "new mail is visible to an IMAP client and pushed over SSE" ($visible.uid -ne "" -and $ev1 -ne "") "send=$($send.Status) imapUid=$($visible.uid) imapWaited=$($visible.waited)s sseFrame=$ev1"

# IMAP-side read state must reach the web side
# The LB round-robins between the two engine nodes, so address the message by
# the UID already resolved above and retry while the nodes converge.
$seen = $null; $tSeen = Get-Date; $evSeen = ""; $seenTries = 0
$fromSeen = Log-Lines $log
while ($seenTries -lt 24 -and (-not $seen -or $seen.status -ne "OK")) {
    $seen = Run-Imap @("seenuid", "$($visible.uid)")
    $seenTries++
    if (-not $seen -or $seen.status -ne "OK") { Start-Sleep -Seconds 5 }
}
$evSeen = Wait-SseFrom $log $fromSeen 'event: mail' 90
$latSeen = [math]::Round(((Get-Date) - $tSeen).TotalSeconds, 1)
Check P14.3b-rev2 "IMAP STORE \Seen pushes a change event to the web side" ($seen.status -eq "OK" -and $evSeen -ne "") "uid=$($seen.uid) status=$($seen.status) tries=$seenTries latency=${latSeen}s"

# ...and so must a move performed by that client
$mv = $null; $tMv = Get-Date; $evMv = ""; $mvTries = 0
$fromMv = Log-Lines $log
while ($mvTries -lt 24 -and (-not $mv -or $mv.status -ne "OK")) {
    $mv = Run-Imap @("moveuid", "$($visible.uid)", "Junk")
    $mvTries++
    if (-not $mv -or $mv.status -ne "OK") { Start-Sleep -Seconds 5 }
}
$evMv = Wait-SseFrom $log $fromMv 'Junk' 120
$latMv = [math]::Round(((Get-Date) - $tMv).TotalSeconds, 1)
Check P14.3c-rev2 "IMAP MOVE Inbox->Junk pushes the folder change" ($mv.status -eq "OK" -and $evMv -ne "") "uid=$($mv.uid) status=$($mv.status) tries=$mvTries latency=${latMv}s frame=$evMv"

# the webmail view converges on the same state (allow the multi-active window)
$junkHit = $false; $flagTxt = ""; $waited = 0
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 180 -and -not $junkHit) {
    $junk = Api $aSess GET "/mail/messages?folder=Junk&page=0"
    foreach ($m in (MsgList $junk)) {
        if ($m.subject -eq $subj) { $junkHit = $true; $flagTxt = ($m.flags -join ",") }
    }
    if (-not $junkHit) { Start-Sleep -Seconds 5 }
}
$waited = [int]$sw.Elapsed.TotalSeconds
$inboxGone = $true
$ib = Api $aSess GET "/mail/messages?folder=Inbox&page=0"
foreach ($m in (MsgList $ib)) { if ($m.subject -eq $subj) { $inboxGone = $false } }
Check P14.3d-rev2 "webmail list agrees with the IMAP move and read state" ($junkHit -and $inboxGone) "junkHasMsg=$junkHit flags=[$flagTxt] inboxGone=$inboxGone convergedIn=${waited}s"

if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Remove-Item -LiteralPath $CookieFile -Force -ErrorAction SilentlyContinue
Flush-Results "P14"
"== done =="
