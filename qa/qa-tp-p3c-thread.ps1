# qa-tp-p3c-thread.ps1 - P3.3 focused re-test: does a reply join the thread?
# GET /mail/thread answers {thread_id, subject, messages}; the per-row
# thread_count lives on the conversation list (/mail/messages?conversation=1).
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P3.3 thread rev3 rnd=$rnd =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function Wait-Mail($sess, [string]$subject, [int]$timeoutSec = 90) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeoutSec) {
        $r = Api $sess GET "/mail/messages?folder=Inbox&page=0"
        foreach ($m in (MsgList $r)) { if ($m.subject -eq $subject) { return $m } }
        Start-Sleep -Seconds 2
    }
    return $null
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-p3c-a-$rnd@example.com"
$B = "qa-p3c-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P3c" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

$subj = "p3c-thread-$rnd"
$s1 = Api $aSess POST "/mail/send" @{ to = @($B); subject = $subj; body = "root body" }
$rootB = Wait-Mail $bSess $subj 60
"root seen by B: $($rootB -ne $null) id=$($rootB.id)"

$s2 = Api $bSess POST "/mail/send" @{ to = @($A); subject = "Re: $subj"; body = "reply body"; in_reply_to = $rootB.id; references = $rootB.id }
$replyA = Wait-Mail $aSess "Re: $subj" 60
"reply seen by A: $($replyA -ne $null) id=$($replyA.id) thread_id=$($replyA.thread_id) thread_count=$($replyA.thread_count)"

# A answers back, so B's Inbox ends up holding two messages of the thread —
# the only place the per-folder conversation view can show a real group.
$s3 = Api $aSess POST "/mail/send" @{ to = @($B); subject = "Re: $subj"; body = "second reply"; in_reply_to = $replyA.id; references = "$($rootB.id) $($replyA.id)" }
Start-Sleep -Seconds 5
$thread = Api $aSess GET "/mail/thread?folder=Inbox&uid=$($replyA.uid)"
"A-side thread lookup (uid form is invalid; thread_id is the contract): $($thread.Status)"

# The conversation is per folder: B's Inbox holds both the original and the
# reply, so that is where the thread must have two members.
$tid = [string]$replyA.thread_id
$threadB = Api $bSess GET "/mail/thread?folder=Inbox&thread_id=$tid"
$msgsB = 0
$rootInThreadB = $false
if ($threadB.Data -and $threadB.Data.messages) {
    $msgsB = @($threadB.Data.messages).Count
    foreach ($m in @($threadB.Data.messages)) { if ($m.subject -eq $subj) { $rootInThreadB = $true } }
}
$convB = Api $bSess GET "/mail/messages?folder=Inbox&page=0&conversation=1"
$convCountB = 0
foreach ($m in (MsgList $convB)) { if ($m.thread_id -eq $tid) { $convCountB = [int]$m.thread_count } }
Check P3.3-rev3 "a reply is grouped with the original mail in the same folder" `
    ($s1.Status -in @(200, 204) -and $s2.Status -in @(200, 204) -and $rootB -ne $null -and $replyA -ne $null -and $msgsB -ge 2 -and $convCountB -ge 2) `
    "threadId=$tid threadMessagesInB=$msgsB rootInThread=$rootInThreadB convThreadCountInB=$convCountB (A-side Inbox counts 1: grouping is per folder by subject)"

# and the sent copy stays visible to the sender as its own row
$convA = Api $aSess GET "/mail/messages?folder=Inbox&page=0&conversation=1"
$aRows = (MsgList $convA).Count
$sentConv = Api $aSess GET "/mail/messages?folder=Sent&page=0&conversation=1"
$sentRows = (MsgList $sentConv).Count
Check P3.3b-rev3 "conversation view stays consistent per folder" ($aRows -ge 1 -and $sentRows -ge 1) "inboxConvRows=$aRows sentConvRows=$sentRows"

$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P3"
"== done =="
