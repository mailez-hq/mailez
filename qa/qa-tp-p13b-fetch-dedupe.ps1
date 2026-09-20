# qa-tp-p13b-fetch-dedupe.ps1 - P13.3/P13.4 re-test with correct judgement
#
# rev1 of the module judged dedupe from the webmail list and the error path
# from last_check. Both are wrong: the list is a second-hand view (and can
# disagree while the two engine nodes converge), and a failed poll never
# advances last_check. This re-test reads the destination over IMAP and watches
# the error column itself.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$FetchHost = if ($env:QA_FETCH_HOST) { $env:QA_FETCH_HOST } else { "engine-lb" }
$Poll = 400
"== P13.3/P13.4 recheck rnd=$rnd host=$FetchHost =="

$admin = New-Session
$PW = "QaUser2026!"
$DST = "qa-p13b-dst-$rnd@example.com"
$SRC = "qa-p13b-src-$rnd@example.com"
foreach ($u in @($DST, $SRC)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P13b" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$srcSess = New-Session $SRC $PW
$subj = "p13b-fetch-$rnd"
[void](Api $srcSess POST "/mail/send" @{ to = @($SRC); subject = $subj; body = "p13b seed" })
Start-Sleep -Seconds 5

function ImapSubjects([string]$email, [string]$pw, [string]$folder = "INBOX") {
    $out = @(& python .\qa-imap-op.py 127.0.0.1 143 $email $pw subjects $folder 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return @() }
    $obj = $json | ConvertFrom-Json
    if (-not $obj.subjects) { return @() }
    return @($obj.subjects | ForEach-Object { $_[1] })
}
function FetchRow([int]$id) {
    $r = Api $admin GET "/fetches?user=$DST&limit=200"
    foreach ($x in @($r.Data.data)) { if ([int]$x.id -eq $id) { return $x } }
    return $null
}

$create = Api $admin POST "/fetches" @{ user_email = $DST; protocol = "pop3"; host = $FetchHost; port = 995; tls = $true; keep = $true; username = $SRC; password = $PW }
$fid = 0
if ($create.Data -and $create.Data.id) { $fid = [int]$create.Data.id }
Check P13.1-rev4 "external POP3 account created (implicit TLS, keep on source)" ($fid -gt 0) "status=$($create.Status) id=$fid"

# ---- first cycle: the message must land in the destination exactly once
$hits = @()
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt $Poll) {
    $hits = @(ImapSubjects $DST $PW | Where-Object { $_ -match [regex]::Escape($subj) })
    if ($hits.Count -gt 0) { break }
    Start-Sleep -Seconds 10
}
$firstWait = [int]$sw.Elapsed.TotalSeconds
Check P13.2-rev4 "poll copies the message into the destination mailbox" ($hits.Count -eq 1) "copies=$($hits.Count) in=${firstWait}s"

# ---- second cycle: nothing may be copied again (same UIDL / same message)
$rowBefore = FetchRow $fid
$advanced = $false
$sw2 = [Diagnostics.Stopwatch]::StartNew()
while ($sw2.Elapsed.TotalSeconds -lt $Poll) {
    Start-Sleep -Seconds 15
    $r2 = FetchRow $fid
    if ($r2 -and $rowBefore -and "$($r2.last_check)" -ne "$($rowBefore.last_check)" -and "$($r2.last_check)" -ne "") { $advanced = $true; break }
}
Start-Sleep -Seconds 15
$hits2 = @(ImapSubjects $DST $PW | Where-Object { $_ -match [regex]::Escape($subj) })
$r3 = FetchRow $fid
Check P13.3-rev4 "second poll cycle delivers no duplicate" ($advanced -and $hits2.Count -eq 1) "cycleAdvanced=$advanced copies=$($hits.Count)->$($hits2.Count) waited=$([int]$sw2.Elapsed.TotalSeconds)s seenUIDLs=$($r3.seen_uidls)"

# ---- bad credentials: the row must surface an error and keep the worker alive
[void](Api $admin PUT "/fetches/$fid" @{ password = "wrong-$rnd-pass" })
$err = ""
$sw3 = [Diagnostics.Stopwatch]::StartNew()
while ($sw3.Elapsed.TotalSeconds -lt $Poll) {
    Start-Sleep -Seconds 15
    $r4 = FetchRow $fid
    if ($r4 -and "$($r4.error)".Trim() -ne "") { $err = "$($r4.error)"; break }
}
$h = Api $admin GET "/health"
Check P13.4-rev4 "bad credentials surface in the fetch row without killing the worker" ($err -ne "" -and $h.Status -eq 200) "error=[$err] health=$($h.Status) waited=$([int]$sw3.Elapsed.TotalSeconds)s"
[void](Api $admin PUT "/fetches/$fid" @{ password = $PW })

$del = Api $admin DELETE "/fetches/$fid"
$gone = $true
$r5 = Api $admin GET "/fetches?user=$DST&limit=200"
foreach ($x in @($r5.Data.data)) { if ([int]$x.id -eq $fid) { $gone = $false } }
Check P13.5-rev4 "deleting the account stops the poller" ($del.Status -in @(200, 204) -and $gone) "del=$($del.Status) gone=$gone"

$ud1 = Api $admin DELETE "/users/$DST"
$ud2 = Api $admin DELETE "/users/$SRC"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P13"
"== done =="
