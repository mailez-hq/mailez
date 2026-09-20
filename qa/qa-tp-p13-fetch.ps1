# qa-tp-p13-fetch.ps1 - P13 external mailbox aggregation (TEST-PROMPTS.md P13), rev3
# rev2: the engine containers only answer behind the engine-lb hop (they expect
# the PROXY protocol, MAILEZINE_PROXY_PROTOCOL=all / trusted=engine-lb), so a
# direct dial to the container name hangs and dies with EOF.
# rev3: the engine also refuses plaintext POP3 AUTH ("TLS required before
# authentication", mailezine/internal/pop3/pop3.go:158) and the backend's
# fetch/POP3 client has no STLS support, so the account must use implicit TLS.
# The stack cert is issued for mail.example.com (+ localhost/127.0.0.1), so the
# poller dials https-style: host=mail.example.com -> engine-lb:995 with
# verification ON. Inside the compose network that name has to be mapped to the
# LB IP first (test-env only):
#   docker exec -u 0 mailez-backend-1 sh -c "echo '192.168.206.250 mail.example.com' >> /etc/hosts"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$FetchHost = if ($env:QA_FETCH_HOST) { $env:QA_FETCH_HOST } else { "mail.example.com" }
$FetchPort = if ($env:QA_FETCH_PORT) { [int]$env:QA_FETCH_PORT } else { 995 }
$FetchTls = if ($env:QA_FETCH_TLS) { $env:QA_FETCH_TLS -eq "true" } else { $true }
"== P13 fetch rev3 rnd=$rnd base=$Base fetchHost=$FetchHost port=$FetchPort tls=$FetchTls =="
$Poll = 380   # backend FETCH_INTERVAL=300s, ticks are phase-shifted
$admin = New-Session
$DST = "qa-p13dst-$rnd@example.com"
$SRC = "qa-p13src-$rnd@example.com"
$PW = "QaUser2026!"
foreach ($u in @($DST, $SRC)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P13" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
# NOTE: PowerShell variable names are case-insensitive — a session bound to
# $src would clobber $SRC (and $dst clobbers $DST), so keep them distinct.
$dstSess = New-Session $DST $PW
$srcSess = New-Session $SRC $PW

# Seed: one message waiting in the source mailbox (the "external" account).
$subject = "p13-fetch-$rnd"
$seed = Api $srcSess POST "/mail/send" @{ to = @($SRC); subject = $subject; body = "p13 seed" }
Check P13.0 "seed message into the source mailbox" ($seed.Status -in @(200, 204)) "send=$($seed.Status)"
Start-Sleep -Seconds 5

# P13.1 add external account (the engine itself acts as the remote)
$create = Api $admin POST "/fetches" @{ user_email = $DST; protocol = "pop3"; host = $FetchHost; port = $FetchPort; tls = $FetchTls; keep = $true; username = $SRC; password = $PW }
$fid = 0
if ($create.Data -and $create.Data.id) { $fid = [int]$create.Data.id }
Check P13.1 "add external account (POP3+TLS via ${FetchHost}:${FetchPort})" ($create.Status -in @(200, 201) -and $fid -gt 0) "status=$($create.Status) id=$fid"

# P13.1b credential at rest must be sealed, not the plaintext password
$cipher = ""
try { $cipher = [string](docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT password FROM ``fetch`` WHERE id=$fid;" 2>$null | Select-Object -First 1) } catch {}
$cipher = "$cipher".Trim()
Check P13.1b "credential sealed in DB (no plaintext)" ($fid -gt 0 -and $cipher -ne "" -and $cipher -ne $PW) "id=$fid len=$($cipher.Length) plaintext=$($cipher -eq $PW)"

function Get-FetchRow {
    $r = Api $admin GET "/fetches?user=$DST"
    foreach ($x in @($r.Data.data)) { if ([int]$x.id -eq $fid) { return $x } }
    return $null
}
function Get-DstMatches {
    $r = Api $dstSess GET "/mail/messages?folder=Inbox&page=0"
    if ($r.Status -ne 200) { return -1 }
    $n = 0
    $items = if ($r.Data.data) { $r.Data.data } else { $r.Data }
    foreach ($m in @($items)) { if ($m.subject -eq $subject) { $n++ } }
    return $n
}

# P13.2 first poll delivers into the target mailbox
$sw = [Diagnostics.Stopwatch]::StartNew()
$seen = 0
$row = $null
while ($sw.Elapsed.TotalSeconds -lt $Poll) {
    $seen = Get-DstMatches
    if ($seen -gt 0) { break }
    Start-Sleep -Seconds 10
}
$row = Get-FetchRow
$firstLatency = [int]$sw.Elapsed.TotalSeconds
Check P13.2 "poll pulls the message into the target Inbox" ($seen -gt 0) "in=${firstLatency}s copies=$seen last_check=$($row.last_check) err=[$($row.error)]"

# P13.3 dedupe across the next poll cycle: same message must not be delivered twice
$sw2 = [Diagnostics.Stopwatch]::StartNew()
$lastSeen = $seen
$advanced = $false
while ($sw2.Elapsed.TotalSeconds -lt $Poll) {
    Start-Sleep -Seconds 15
    $r2 = Get-FetchRow
    if ($r2 -and $row -and $r2.last_check -ne $row.last_check) { $advanced = $true; break }
}
Start-Sleep -Seconds 20
$seen2 = Get-DstMatches
Check P13.3 "dedupe: no duplicate on the next cycle" ($advanced -and $seen2 -eq $seen) "cycleAdvanced=$advanced copies=$seen->$seen2 waited=$([int]$sw2.Elapsed.TotalSeconds)s"

# P13.4 broken credentials surface as an error without killing the worker
[void](Api $admin PUT "/fetches/$fid" @{ password = "wrong-$rnd-pass" })
$sw3 = [Diagnostics.Stopwatch]::StartNew()
$err = ""
$before = ""
if ($row) { $before = "$($row.last_check)" }
while ($sw3.Elapsed.TotalSeconds -lt $Poll) {
    Start-Sleep -Seconds 15
    $r3 = Get-FetchRow
    if ($r3 -and $r3.last_check -ne $before) { $err = "$($r3.error)"; break }
}
$h = Api $admin GET "/health"
Check P13.4 "bad credentials: clear error, worker alive" ($err -ne "") "error=[$err] backendHealth=$($h.Status)"
[void](Api $admin PUT "/fetches/$fid" @{ password = $PW })

# P13.5 delete stops the poller and removes the account
$del = Api $admin DELETE "/fetches/$fid"
$gone = $true
$r5 = Api $admin GET "/fetches?user=$DST"
foreach ($x in @($r5.Data.data)) { if ([int]$x.id -eq $fid) { $gone = $false } }
Check P13.5 "delete stops polling and removes the account" ($del.Status -in @(200, 204) -and $gone) "del=$($del.Status) gone=$gone"

# P13.6 the internal API must not hand out credentials unauthenticated
$stackBase = $Base -replace "/api/v1$", ""
$code = 0
try { $r6 = Invoke-WebRequest -Uri "$stackBase/stack/fetch" -Method GET -UseBasicParsing -TimeoutSec 15; $code = [int]$r6.StatusCode } catch { if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode } }
Check P13.6 "GET /stack/fetch without STACK_SECRET is refused" ($code -in @(401, 403)) "status=$code"

$ud1 = Api $admin DELETE "/users/$DST"
$ud2 = Api $admin DELETE "/users/$SRC"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P13"
"== done =="
