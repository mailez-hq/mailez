# qa-trash-purge-recheck.ps1 - live recheck of DEF-TRASH-PURGE:
# a message deleted inside Trash must actually disappear (purge), not stay.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa

$rnd = Get-Random -Maximum 999999
$env:QA_BASE = "http://127.0.0.1:8081/api/v1"
Import-Module .\qa-lib.psm1 -Force
"== trash purge recheck rnd=$rnd =="

function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}

$PW = "QaUser2026!"
$A = "qa-tp-a-$rnd@example.com"
$B = "qa-tp-b-$rnd@example.com"
$admin = New-Session
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA tp" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

# deliver one message to B
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = "tp-purge-$rnd"; body = "purge me" })
$deadline = (Get-Date).AddSeconds(60)
$uid = 0
while ((Get-Date) -lt $deadline) {
    $rows = @(MsgList (Api $bSess GET "/mail/messages?folder=Inbox&page=0") | Where-Object { $_.subject -match "tp-purge-$rnd" })
    if ($rows.Count -ge 1) { $uid = $rows[0].uid; break }
    Start-Sleep -Seconds 3
}
if ($uid -eq 0) { "FAIL deliver (message never visible)"; exit 1 }
"delivered uid=$uid"

# step 1: delete from Inbox -> should move to Trash
$r = Api $bSess POST "/mail/delete" @{ folder = "Inbox"; uid = $uid }
"delete-in-inbox -> $($r.Status) (expect 204)"
Start-Sleep -Seconds 5
$trash = @(MsgList (Api $bSess GET "/mail/messages?folder=Trash&page=0") | Where-Object { $_.subject -match "tp-purge-$rnd" })
"trash count after move = $($trash.Count) (expect 1)"

# step 2: delete inside Trash -> must purge
$r = Api $bSess POST "/mail/delete" @{ folder = "Trash"; uid = $trash[0].uid }
"delete-in-trash -> $($r.Status) (expect 204)"

$gone = $false
$deadline = (Get-Date).AddSeconds(120)
while ((Get-Date) -lt $deadline) {
    $trash = @(MsgList (Api $bSess GET "/mail/messages?folder=Trash&page=0") | Where-Object { $_.subject -match "tp-purge-$rnd" })
    if ($trash.Count -eq 0) { $gone = $true; break }
    Start-Sleep -Seconds 10
}
if ($gone) { "PASS message purged from Trash" } else { "FAIL message still in Trash after 120s (count=$($trash.Count))" }

# cleanup
foreach ($u in @($A, $B)) {
    $d = Api $admin DELETE "/users/$u"
    if ($d.Status -notin @(200, 204)) { "cleanup: delete $u -> $($d.Status) $($d.Body)" }
}
"cleanup done"


