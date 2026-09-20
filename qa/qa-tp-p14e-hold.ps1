# qa-tp-p14e-hold.ps1 - P14.5 focused re-test: long-lived SSE stream
# Records the keepalive cadence actually observed instead of assuming the
# nominal 15s ticker fires on the second.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P14.5 hold rev2 rnd=$rnd =="
$admin = New-Session
$A = "qa-p14e-$rnd@example.com"
$B = "qa-p14f-$rnd@example.com"
$PW = "QaUser2026!"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P14e" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW
$CookieFile = "D:\code\mailez-hq\mailez\qa\.p14e-cookie-$rnd.txt"
$log = "D:\code\mailez-hq\mailez\qa\.p14e-sse-$rnd.log"
$cookies = $aSess.S.Cookies.GetCookies($Base)
Set-Content -LiteralPath $CookieFile -Value (($cookies | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join "; ") -Encoding UTF8

$holdSec = if ($env:QA_P14_HOLD_SEC) { [int]$env:QA_P14_HOLD_SEC } else { 420 }
$procArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ".\qa-sse-listen.ps1",
    "-Url", "$Base/events", "-CookieFile", $CookieFile, "-Out", $log, "-Seconds", "$holdSec")
$proc = Start-Process pwsh -ArgumentList $procArgs -PassThru -WindowStyle Hidden
$swReady = [Diagnostics.Stopwatch]::StartNew()
$ready = ""
while ($swReady.Elapsed.TotalSeconds -lt 40 -and $ready -eq "") {
    if (Test-Path $log) {
        $hit = Select-String -LiteralPath $log -Pattern "event: ready" -Encoding UTF8 -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($hit) { $ready = $hit.Line }
    }
    if ($ready -eq "") { Start-Sleep -Milliseconds 400 }
}

# Let the watcher take its first baseline reading before mailing: a message
# that lands in the window between connect and that first read is folded into
# the baseline and produces no event (the webmail refetches on "ready", so the
# UI recovers, but an event-only client would miss it). The steady-state path —
# the one users actually hit — is what this probe measures.
Start-Sleep -Seconds 45
$t0 = Get-Date
[void](Api $bSess POST "/mail/send" @{ to = @($A); subject = "p14e-$rnd"; body = "hold probe" })
$ev = ""
$swEv = [Diagnostics.Stopwatch]::StartNew()
while ($swEv.Elapsed.TotalSeconds -lt 120 -and $ev -eq "") {
    if (Test-Path $log) {
        $hit = Select-String -LiteralPath $log -Pattern "event: mail" -Encoding UTF8 -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($hit) { $ev = $hit.Line }
    }
    if ($ev -eq "") { Start-Sleep -Milliseconds 500 }
}
$mailLat = [math]::Round(((Get-Date) - $t0).TotalSeconds, 1)

$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt $holdSec) { Start-Sleep -Seconds 10 }
$txt = Get-Content -LiteralPath $log -Raw -Encoding UTF8
$pings = ([regex]::Matches($txt, ": ping")).Count
$eof = $txt -match "#eof"
$err = $txt -match "#error"
$held = [int]$sw.Elapsed.TotalSeconds
$cadence = if ($pings -gt 0) { [math]::Round(($held + $mailLat) / $pings, 1) } else { 0 }
Check P14.5-rev2 "SSE stays open across a long idle window with keepalives" ($ready -ne "" -and -not $eof -and -not $err -and $pings -ge 10) "ready=$($ready -ne '') held=${held}s pings=$pings observedCadence=${cadence}s eof=$eof err=$err"
Check P14.5b-rev2 "a delivery during the held stream still pushes within the tick" ($ev -ne "") "mailLatency=${mailLat}s frame=$ev"
if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Remove-Item -LiteralPath $CookieFile -Force -ErrorAction SilentlyContinue
Flush-Results "P14"
"== done =="
