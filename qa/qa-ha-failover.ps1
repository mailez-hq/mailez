# qa-ha-failover.ps1 - multi-active spot check: kill one engine node, keep
# using the stack, bring it back, then verify nothing was lost or duplicated.
#
# The stack runs two mailezine instances behind engine-lb (PROXY protocol).
# This stops the second one, drives real mail through the surviving node, then
# restarts it and checks convergence. Everything is read through the same
# paths customers use (webmail API + IMAP).
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$NODE = if ($env:QA_ENGINE_NODE) { $env:QA_ENGINE_NODE } else { "mailez-mailezine-2" }
"== HA failover probe rnd=$rnd node=$NODE =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function ImapSubjects([string]$email, [string]$pw, [string]$folder = "INBOX") {
    $out = @(& python .\qa-imap-op.py 127.0.0.1 143 $email $pw subjects $folder 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return @() }
    $obj = $json | ConvertFrom-Json
    if (-not $obj.subjects) { return @() }
    return @($obj.subjects | ForEach-Object { $_[1] })
}
function WaitSubject([string]$email, [string]$pw, [string]$subject, [int]$timeout = 120) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeout) {
        $subs = ImapSubjects $email $pw
        if ($subs -match [regex]::Escape($subject)) { return [int]$sw.Elapsed.TotalSeconds }
        Start-Sleep -Seconds 3
    }
    return -1
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-ha-a-$rnd@example.com"
$B = "qa-ha-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA HA" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW

# --- baseline: both nodes up
$s1 = "ha-baseline-$rnd"
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = $s1; body = "baseline" })
$baseSecs = WaitSubject $B $PW $s1 90
Check HA.1 "delivery works with both engine nodes up" ($baseSecs -ge 0) "deliveredIn=${baseSecs}s"

# --- kill one node
$stopOut = docker stop $NODE 2>&1 | Out-String
Start-Sleep -Seconds 12
$lbHealth = docker ps --format "{{.Names}} {{.Status}}" | Select-String -Pattern "mailez-mailezine"
"node state after stop: $($lbHealth -join ' | ')"

$s2 = "ha-degraded-$rnd"
$send2 = Api $aSess POST "/mail/send" @{ to = @($B); subject = $s2; body = "degraded" }
$degSecs = WaitSubject $B $PW $s2 150
Check HA.2 "mail is still delivered with one engine node down" ($send2.Status -in @(200, 204) -and $degSecs -ge 0) `
    "send=$($send2.Status) deliveredIn=${degSecs}s node=$NODE stopped"

# reads must keep working too (list + detail through the LB)
$bSess = New-Session $B $PW
$list = Api $bSess GET "/mail/messages?folder=Inbox&page=0"
$rows = MsgList $list
$seenBoth = (@($rows | Where-Object { $_.subject -eq $s1 }).Count -ge 1) -and (@($rows | Where-Object { $_.subject -eq $s2 }).Count -ge 1)
Check HA.3 "webmail list still serves both messages while degraded" ($list.Status -eq 200 -and $seenBoth) `
    "status=$($list.Status) rows=$($rows.Count) hasBaseline=$(@($rows | Where-Object { $_.subject -eq $s1 }).Count) hasDegraded=$(@($rows | Where-Object { $_.subject -eq $s2 }).Count)"

# --- bring the node back
$startOut = docker start $NODE 2>&1 | Out-String
Start-Sleep -Seconds 25
$s3 = "ha-restored-$rnd"
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = $s3; body = "restored" })
$resSecs = WaitSubject $B $PW $s3 150
Check HA.4 "delivery works again after the node returns" ($resSecs -ge 0) "deliveredIn=${resSecs}s nodeStarted=$($startOut.Trim())"

# --- convergence: every message exactly once, on both the API and IMAP views
Start-Sleep -Seconds 20
$apiRows = MsgList (Api $bSess GET "/mail/messages?folder=Inbox&page=0")
$imapSubs = ImapSubjects $B $PW
$counts = @{}
foreach ($s in @($s1, $s2, $s3)) {
    $counts[$s] = @($apiRows | Where-Object { $_.subject -eq $s }).Count
}
$imapCounts = @{}
foreach ($s in @($s1, $s2, $s3)) {
    $imapCounts[$s] = @($imapSubs | Where-Object { $_ -match [regex]::Escape($s) }).Count
}
$apiOk = (@($counts.Values | Where-Object { $_ -eq 1 }).Count -eq 3)
$imapOk = (@($imapCounts.Values | Where-Object { $_ -eq 1 }).Count -eq 3)
Check HA.5 "after recovery every message appears exactly once (no loss, no duplicate)" ($apiOk -and $imapOk) `
    "api counts: base=$($counts[$s1]) degraded=$($counts[$s2]) restored=$($counts[$s3]); imap counts: base=$($imapCounts[$s1]) degraded=$($imapCounts[$s2]) restored=$($imapCounts[$s3])"

$nodes = docker ps --format "{{.Names}} {{.Status}}" | Select-String -Pattern "mailez-mailezine"
"final node state: $($nodes -join ' | ')"
$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "HA"
"== done =="
