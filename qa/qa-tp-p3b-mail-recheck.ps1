# qa-tp-p3b-mail-recheck.ps1 - re-run the P3 cases that the first pass marked
# "blocked by DEF-P3-VIS" (recipient list visibility), rev1
#
# Why: /mail/messages answers with a bare JSON array. Testing the page-envelope
# with `if ($r.Data.data)` is unsafe in PowerShell — for an array of N>1 rows
# every element lacks .data, member enumeration yields N $nulls, and the array
# is still truthy, so the caller ends up iterating nulls. That failure mode
# matches the first pass exactly: a mailbox holding ONE message parsed fine
# (single null is falsy, the fallback branch ran) while a mailbox holding two
# or more looked empty until something else happened to make it parse.
# This script parses the array (and the X-Total-Messages header) explicitly.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P3 recheck rev1 rnd=$rnd base=$Base =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function Find-Mail($sess, [string]$subject, [string]$folder = "Inbox") {
    $r = Api $sess GET "/mail/messages?folder=$folder&page=0"
    foreach ($m in (MsgList $r)) { if ($m.subject -eq $subject) { return $m } }
    return $null
}
function Wait-Mail($sess, [string]$subject, [int]$timeoutSec = 90, [string]$folder = "Inbox") {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeoutSec) {
        $m = Find-Mail $sess $subject $folder
        if ($m) { return @{ msg = $m; waited = [Math]::Round($sw.Elapsed.TotalSeconds, 1) } }
        Start-Sleep -Seconds 2
    }
    return @{ msg = $null; waited = [Math]::Round($sw.Elapsed.TotalSeconds, 1) }
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-p3b-a-$rnd@example.com"
$B = "qa-p3b-b-$rnd@example.com"
$C = "qa-p3b-c-$rnd@example.com"
$D = "qa-p3b-d-$rnd@example.com"
foreach ($u in @($A, $B, $C, $D)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P3b" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW
$cSess = New-Session $C $PW
$dSess = New-Session $D $PW

# --- the case DEF-P3-VIS was built on: several messages, read immediately
$lat = @()
$allSeen = $true
for ($i = 1; $i -le 3; $i++) {
    $subj = "p3b-$rnd-m$i"
    $s = Api $aSess POST "/mail/send" @{ to = @($B); subject = $subj; body = "p3b body $i" }
    $w = Wait-Mail $bSess $subj 60
    $lat += "m$i=$($w.waited)s"
    if (-not $w.msg) { $allSeen = $false }
}
Check P3.1-rev2 "successive messages are visible in the recipient list right away" $allSeen "latencies=[$($lat -join ' ')]"

# --- P3.3 reply joins a conversation
$rootSubj = "p3b-thread-$rnd"
$r1 = Api $aSess POST "/mail/send" @{ to = @($B); subject = $rootSubj; body = "root" }
$wRoot = Wait-Mail $bSess $rootSubj 60
$rootMsg = $wRoot.msg
$r2 = $null
if ($rootMsg) {
    $r2 = Api $bSess POST "/mail/send" @{ to = @($A); subject = "Re: $rootSubj"; body = "reply"; in_reply_to = $rootMsg.id; references = $rootMsg.id }
}
$wReply = Wait-Mail $aSess "Re: $rootSubj" 60
$threadCount = 0
if ($wReply.msg) {
    $t = Api $aSess GET "/mail/thread?folder=Inbox&uid=$($wReply.msg.uid)"
    if ($t.Data -and $t.Data.thread_count) { $threadCount = [int]$t.Data.thread_count }
}
Check P3.3-rev2 "reply joins the conversation (thread_count >= 2)" ($rootMsg -ne $null -and $r2.Status -in @(200, 204) -and $threadCount -ge 2) "rootSeenIn=$($wRoot.waited)s replySeenIn=$($wReply.waited)s threadCount=$threadCount"

# --- P3.4a small attachment round trip, byte identical
$attBytes = [byte[]](1..1024 | ForEach-Object { $_ % 251 })
$attB64 = [Convert]::ToBase64String($attBytes)
$subjAtt = "p3b-att-$rnd"
$sendAtt = Api $aSess POST "/mail/send" @{
    to = @($B); subject = $subjAtt; body = "with attachment"
    attachments = @(@{ name = "p3b-small.txt"; type = "text/plain"; data = $attB64 })
}
$wAtt = Wait-Mail $bSess $subjAtt 90
$attOk = $false; $attDetail = ""
if ($wAtt.msg) {
    $md = Api $bSess GET "/mail/message?folder=Inbox&uid=$($wAtt.msg.uid)"
    if ($md.Data -and $md.Data.attachments -and @($md.Data.attachments).Count -ge 1) {
        $a0 = @($md.Data.attachments)[0]
        if ($a0.data) {
            $got = [Convert]::FromBase64String(($a0.data -replace '^data:[^;]+;base64,', ''))
            $same = ($got.Length -eq $attBytes.Length)
            if ($same) {
                for ($k = 0; $k -lt $got.Length; $k++) { if ($got[$k] -ne $attBytes[$k]) { $same = $false; break } }
            }
            $attOk = $same
            $attDetail = "sent=$($attBytes.Length) got=$($got.Length) same=$same name=$($a0.name)"
        }
    }
}
Check P3.4a-rev2 "small attachment round trip is byte identical" ($sendAtt.Status -in @(200, 204) -and $attOk) "send=$($sendAtt.Status) seenIn=$($wAtt.waited)s $attDetail"

# --- P3.4b large attachment is delivered intact
$bigLen = 14 * 1024 * 1024
$block = New-Object byte[] (1MB)
for ($i = 0; $i -lt $block.Length; $i++) { $block[$i] = [byte]($i % 251) }
$big = New-Object byte[] $bigLen
for ($i = 0; $i -lt ($bigLen / $block.Length); $i++) { [Array]::Copy($block, 0, $big, $i * $block.Length, $block.Length) }
$subjBig = "p3b-big-$rnd"
$sendBig = Api $aSess POST "/mail/send" @{ to = @($B); subject = $subjBig; body = "big attachment"; attachments = @(@{ name = "p3b-big.txt"; type = "text/plain"; data = [Convert]::ToBase64String($big) }) }
$wBig = Wait-Mail $bSess $subjBig 180
$bigSize = 0
if ($wBig.msg) {
    $mdB = Api $bSess GET "/mail/message?folder=Inbox&uid=$($wBig.msg.uid)"
    if ($mdB.Data -and $mdB.Data.attachments) { $bigSize = [int](@($mdB.Data.attachments)[0].size) }
}
Check P3.4b-rev2 "14MB attachment is delivered with the right size" ($sendBig.Status -in @(200, 204) -and $bigSize -eq $bigLen) "send=$($sendBig.Status) seenIn=$($wBig.waited)s size=$bigSize/$bigLen"

# --- P3.5 Bcc is delivered but stripped from the To copy
$subjBcc = "p3b-bcc-$rnd"
$sendBcc = Api $aSess POST "/mail/send" @{ to = @($B); bcc = @($C); subject = $subjBcc; body = "bcc probe" }
$wB = Wait-Mail $bSess $subjBcc 90
$wC = Wait-Mail $cSess $subjBcc 90
$bccHeaderGone = $true
if ($wB.msg) {
    $rawB = Api $bSess GET "/mail/raw?folder=Inbox&uid=$($wB.msg.uid)"
    if ($rawB.Data -and $rawB.Data.raw) { $bccHeaderGone = -not ([string]$rawB.Data.raw -match "(?m)^Bcc:") }
}
Check P3.5-rev2 "Bcc recipient receives the mail and the To copy hides Bcc" ($sendBcc.Status -in @(200, 204) -and $wB.msg -ne $null -and $wC.msg -ne $null -and $bccHeaderGone) "send=$($sendBcc.Status) toSeen=$($wB.msg -ne $null) bccSeen=$($wC.msg -ne $null) bccHeaderStripped=$bccHeaderGone"

# --- P3.6 send as alias
$alias = "qa-p3b-alias-$rnd@example.com"
$mkAlias = Api $admin POST "/aliases" @{ email = $alias; destination = $A; name = "P3b alias" }
$subjAlias = "p3b-alias-$rnd"
$sendAlias = Api $aSess POST "/mail/send" @{ from = $alias; to = @($C); subject = $subjAlias; body = "sent as alias" }
$wAlias = Wait-Mail $cSess $subjAlias 90
$fromOk = $false
if ($wAlias.msg) {
    $rawC = Api $cSess GET "/mail/raw?folder=Inbox&uid=$($wAlias.msg.uid)"
    if ($rawC.Data -and $rawC.Data.raw) { $fromOk = ([string]$rawC.Data.raw -match "(?m)^From:.*$([regex]::Escape($alias))") }
}
$spoof = Api $aSess POST "/mail/send" @{ from = "someone-else-$rnd@example.com"; to = @($C); subject = "p3b-spoof-$rnd"; body = "spoof" }
Check P3.6-rev2 "alias send works and sending as a foreign identity is refused" ($mkAlias.Status -in @(200, 201) -and $sendAlias.Status -in @(200, 204) -and $fromOk -and $spoof.Status -eq 403) "aliasCreate=$($mkAlias.Status) sendAsAlias=$($sendAlias.Status) fromHeader=$fromOk spoof=$($spoof.Status)"

# --- P3.8 fan-out To(2)+Cc(1)
$subjFan = "p3b-fan-$rnd"
$sendFan = Api $aSess POST "/mail/send" @{ to = @($B, $C); cc = @($D); subject = $subjFan; body = "fan out" }
$wB2 = Wait-Mail $bSess $subjFan 90
$wC2 = Wait-Mail $cSess $subjFan 90
$wD2 = Wait-Mail $dSess $subjFan 90
Check P3.8-rev2 "fan-out delivers to both To recipients and the Cc recipient" ($sendFan.Status -in @(200, 204) -and $wB2.msg -ne $null -and $wC2.msg -ne $null -and $wD2.msg -ne $null) "send=$($sendFan.Status) B=$($wB2.waited)s C=$($wC2.waited)s D=$($wD2.waited)s"

$cleanup = @()
$cleanup += "alias=" + (Api $admin DELETE "/aliases/$alias").Status
foreach ($u in @($A, $B, $C, $D)) { $cleanup += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cleanup -join ' ')"
Flush-Results "P3"
"== done =="
