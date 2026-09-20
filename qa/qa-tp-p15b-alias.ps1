# qa-tp-p15b-alias.ps1 - focused re-test: does an alias actually deliver?
# Reads the destination mailbox straight over IMAP (bypassing the webmail list
# and its convergence window) and pulls the engine log for the same message.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P15.4 alias delivery rev2 rnd=$rnd =="

$admin = New-Session
$PW = "QaUser2026!"
$DST = "qa-p15b-dst-$rnd@example.com"
$SRC = "qa-p15b-src-$rnd@example.com"
$AL = "qa-p15b-alias-$rnd@example.com"
foreach ($u in @($DST, $SRC)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P15b" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$srcSess = New-Session $SRC $PW

$mk = Api $admin POST "/aliases" @{ email = $AL; destination = $DST; name = "P15b alias" }
$row = ""
try { $row = (docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT CONCAT(email,' -> ',destination) FROM alias WHERE email='$AL';" 2>$null | Select-Object -First 1) } catch {}
Check P15.4c-1 "alias row created with the announced destination" ($mk.Status -in @(200, 201) -and "$row".Trim() -eq "$AL -> $DST") "create=$($mk.Status) row=$("$row".Trim())"

$subj = "p15b-alias-$rnd"
$send = Api $srcSess POST "/mail/send" @{ to = @($AL); subject = $subj; body = "alias probe body" }
"send=$($send.Status)"

# IMAP is the most direct read of the destination mailbox
$hit = ""
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 180 -and $hit -eq "") {
    $out = @(& python .\qa-imap-op.py 127.0.0.1 143 $DST $PW subjects INBOX 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if ($json -and $json -match [regex]::Escape($subj)) { $hit = $json }
    if ($hit -eq "") { Start-Sleep -Seconds 5 }
}
$waited = [int]$sw.Elapsed.TotalSeconds
Check P15.4c-2 "mail addressed to the alias lands in the destination mailbox (IMAP)" ($send.Status -in @(200, 204) -and $hit -ne "") "in=${waited}s evidence=$hit"

$elog = docker logs mailez-mailezine-1 --since 6m 2>&1 | Out-String
$envLog = docker logs mailez-mailezine-2 --since 6m 2>&1 | Out-String
$expanded = (($elog + $envLog) -match [regex]::Escape($AL))
Check P15.4c-3 "engine log shows the alias expansion for that message" $expanded "aliasMentioned=$expanded"

$del = Api $admin DELETE "/aliases/$AL"
$ud1 = Api $admin DELETE "/users/$DST"
$ud2 = Api $admin DELETE "/users/$SRC"
"cleanup: alias=$($del.Status) users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P15"
"== done =="
