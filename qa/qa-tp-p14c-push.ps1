# qa-tp-p14c-push.ps1 - Web Push fan-out re-test (P14.2c), rev1
#
# The first P14 run subscribed and mailed 7s later; the notifier's delivery
# receipt primed its unseen baseline from that delivery and never raised a
# push. This probe separates the two cases:
#   A) mail arrives BEFORE the notifier has ever read a baseline for the user
#      (the race the first run hit)
#   B) mail arrives AFTER the baseline exists (subscribe -> wait a poll tick ->
#      mail), which is the steady-state path users see
# The push endpoint is deliberately unresolvable, so a dispatch shows up in the
# backend log as "push: notify <user>: ..." — that line is the evidence.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P14.2 push fan-out rev2 rnd=$rnd =="

$admin = New-Session
$A = "qa-p14d-a-$rnd@example.com"
$B = "qa-p14d-b-$rnd@example.com"
$PW = "QaUser2026!"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P14d" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

function Push-Attempts([string]$email, [string]$since = "5m") {
    $tail = docker logs mailez-backend-1 --since $since 2>&1 | Out-String
    $hits = [regex]::Matches($tail, "push: notify $([regex]::Escape($email))[^\r\n]*")
    return $hits.Count
}

$endpoint = "https://qa-push.invalid/p14d-$rnd"
$sub = Api $aSess POST "/push/subscribe" @{ endpoint = $endpoint; keys = @{ p256dh = "BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U"; auth = "I3x4u1zqk8Yy2w2m1n5Q0A" } }
Check P14.2c-1 "subscribe (fresh account) stores the endpoint" ($sub.Status -eq 204) "status=$($sub.Status)"

# --- A) the race: mail lands before the notifier ever read a baseline
$sendA = Api $bSess POST "/mail/send" @{ to = @($A); subject = "p14d-race-$rnd"; body = "race body" }
$hitsA = 0
$swA = [Diagnostics.Stopwatch]::StartNew()
while ($swA.Elapsed.TotalSeconds -lt 150) {
    $hitsA = Push-Attempts $A "4m"
    if ($hitsA -gt 0) { break }
    Start-Sleep -Seconds 15
}
"case A: send=$($sendA.Status) pushAttempts=$hitsA after $([int]$swA.Elapsed.TotalSeconds)s"

# --- B) steady state: let the poll tick establish the baseline first
$settle = if ($env:QA_P14_SETTLE_SEC) { [int]$env:QA_P14_SETTLE_SEC } else { 90 }
Start-Sleep -Seconds $settle
$beforeB = Push-Attempts $A "4m"
$sendB = Api $bSess POST "/mail/send" @{ to = @($A); subject = "p14d-steady-$rnd"; body = "steady body" }
$hitsB = $beforeB
$swB = [Diagnostics.Stopwatch]::StartNew()
while ($swB.Elapsed.TotalSeconds -lt 180) {
    $hitsB = Push-Attempts $A "4m"
    if ($hitsB -gt $beforeB) { break }
    Start-Sleep -Seconds 15
}
Check P14.2c-2 "new mail (after the baseline exists) triggers the push fan-out" ($sendB.Status -in @(200, 204) -and $hitsB -gt $beforeB) "send=$($sendB.Status) attemptsBefore=$beforeB attemptsAfter=$hitsB in=$([int]$swB.Elapsed.TotalSeconds)s"
Check P14.2c-3 "first mail after subscribe is not swallowed by baseline priming" ($hitsA -gt 0) "caseA=send=$($sendA.Status) pushAttempts=$hitsA caseB attempts=$hitsB"

$unsub = Api $aSess DELETE "/push/subscribe" @{ endpoint = $endpoint }
$after = 0
try { $after = [int](docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT COUNT(*) FROM push_subscription WHERE user_email='$A';" 2>$null | Select-Object -First 1) } catch {}
Check P14.2d-rev2 "unsubscribe removes the endpoint" ($unsub.Status -in @(200, 204) -and $after -eq 0) "status=$($unsub.Status) rowsAfter=$after"

$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P14"
"== done =="
