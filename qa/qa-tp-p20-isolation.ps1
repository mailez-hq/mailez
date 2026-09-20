# qa-tp-p20-isolation.ps1 - P20 horizontal isolation, IDOR, unauthenticated
# access and injection surface (TEST-PROMPTS.md P20), rev1
#
# Harmless checks only: everything reads or attempts writes on records owned by
# another test account and asserts the API refuses without leaking data.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P20 isolation rev1 rnd=$rnd base=$Base =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function Raw($resp) {
    if ($resp.Raw) { return ([Text.Encoding]::UTF8.GetString($resp.Raw.RawContentStream.ToArray())) }
    return ""
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-p20-a-$rnd@example.com"
$B = "qa-p20-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P20" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
# PowerShell variable names are case-insensitive: sessions must not be bound
# to $a/$b or they overwrite the $A/$B addresses.
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

# ---- B seeds one record of every kind, each carrying a unique marker
$marker = "P20SECRET$rnd"
$subjB = "p20-mail-$rnd"
[void](Api $bSess POST "/mail/send" @{ to = @($B); subject = $subjB; body = "$marker mail body" })
[void](Api $bSess POST "/mail/draft" @{ to = @("nobody@example.com"); subject = "p20-draft-$rnd"; body = "$marker draft body" })

$bFileId = 0
$tmpFile = Join-Path $env:TEMP "p20-$rnd.txt"
Set-Content -LiteralPath $tmpFile -Value "$marker drive body" -Encoding UTF8
try {
    $up = Invoke-WebRequest -Uri "$Base/drive/upload" -Method POST -WebSession $bSess.S -Form @{ file = Get-Item $tmpFile } -UseBasicParsing -TimeoutSec 60
    $upJson = [Text.Encoding]::UTF8.GetString($up.RawContentStream.ToArray()) | ConvertFrom-Json
    if ($upJson.id) { $bFileId = [int]$upJson.id }
} catch {
    "drive upload failed: $($_.Exception.Message)"
}
if ($bFileId -eq 0) {
    $tree = Api $bSess GET "/drive/tree?parent_id="
    foreach ($f in (MsgList $tree)) { if ($f.name -eq "p20-$rnd.txt") { $bFileId = [int]$f.id } }
}

$bContactId = 0
$c = Api $bSess POST "/contacts" @{ name = "P20 Prober"; email = "p20-$rnd@example.com"; comment = $marker }
if ($c.Data -and $c.Data.id) { $bContactId = [int]$c.Data.id }

$bEventId = 0
$ev = Api $bSess POST "/calendar/events" @{ summary = "p20-$rnd"; start = (Get-Date).AddDays(3).ToString("o"); end = (Get-Date).AddDays(3).AddHours(1).ToString("o"); location = $marker }
if ($ev.Data -and $ev.Data.id) { $bEventId = [int]$ev.Data.id }

$bUid = 0
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 60 -and $bUid -eq 0) {
    $l = Api $bSess GET "/mail/messages?folder=Inbox&page=0"
    foreach ($m in (MsgList $l)) { if ($m.subject -eq $subjB) { $bUid = [int]$m.uid } }
    if ($bUid -eq 0) { Start-Sleep -Seconds 3 }
}
"seeded: bUid=$bUid driveId=$bFileId contactId=$bContactId eventId=$bEventId"

# ------------------------------------------- P20.1 horizontal isolation: mail
$readB = Api $aSess GET "/mail/message?folder=Inbox&uid=$bUid"
$rawB = Api $aSess GET "/mail/raw?folder=Inbox&uid=$bUid"
$leak = (Raw $readB) -match $marker -or (Raw $rawB) -match $marker
Check P20.1a "another session cannot read B's message" (-not $leak) "message=$($readB.Status) raw=$($rawB.Status) markerLeaked=$leak"

[void](Api $aSess POST "/mail/flag" @{ folder = "Inbox"; uid = $bUid; flag = "flagged"; value = $true })
[void](Api $aSess POST "/mail/delete" @{ folder = "Inbox"; uid = $bUid })
[void](Api $aSess POST "/mail/move" @{ folder = "Inbox"; uid = $bUid; destination = "Junk" })
Start-Sleep -Seconds 2
$stillThere = $false
$l2 = Api $bSess GET "/mail/messages?folder=Inbox&page=0"
foreach ($m in (MsgList $l2)) { if ($m.subject -eq $subjB) { $stillThere = $true } }
Check P20.1b "A's flag/delete/move calls leave B's message intact" $stillThere "messageStillInBInbox=$stillThere"

# ---------------------------------------------- isolation: drive and contacts
$dl = Api $aSess GET "/drive/download/$bFileId"
$dlLeak = (Raw $dl) -match $marker
Check P20.1c "A cannot download B's drive file" (($dl.Status -in @(403, 404)) -and -not $dlLeak) "status=$($dl.Status) markerLeaked=$dlLeak"

$ctUpd = Api $aSess PUT "/contacts/$bContactId" @{ name = "hijacked"; email = "x@example.com" }
$ctDel = Api $aSess DELETE "/contacts/$bContactId"
$ctRead = Api $bSess GET "/contacts?page=1&limit=200"
$ctIntact = $false
foreach ($x in (MsgList $ctRead)) { if ($x.comment -eq $marker -and $x.name -eq "P20 Prober") { $ctIntact = $true } }
Check P20.1d "A cannot modify or delete B's contact" ($ctUpd.Status -in @(403, 404) -and $ctIntact) "update=$($ctUpd.Status) delete=$($ctDel.Status) contactIntact=$ctIntact"

$evUpd = Api $aSess PUT "/calendar/events/$bEventId" @{ summary = "hijacked" }
$evDel = Api $aSess DELETE "/calendar/events/$bEventId"
$evList = Api $bSess GET "/calendar/events"
$evIntact = $false
foreach ($x in (MsgList $evList)) { if ($x.location -eq $marker -and $x.summary -eq "p20-$rnd") { $evIntact = $true } }
Check P20.1e "A cannot modify or delete B's calendar event" ($evUpd.Status -in @(403, 404) -and $evIntact) "update=$($evUpd.Status) delete=$($evDel.Status) eventIntact=$evIntact"

# ------------------------------------------------------- P20.4 IDOR sweep
$leaks = @()
for ($id = 1; $id -le 60; $id++) {
    if ($id -eq $bFileId) { continue }
    $r = Api $aSess GET "/drive/download/$id"
    if ($r.Status -eq 200 -and (Raw $r) -match $marker) { $leaks += "drive:$id" }
}
for ($id = 1; $id -le 60; $id++) {
    if ($id -eq $bContactId) { continue }
    $r = Api $aSess GET "/contacts/$id"
    if ($r.Status -eq 200 -and (Raw $r) -match $marker) { $leaks += "contact:$id" }
}
Check P20.4 "sequential id enumeration leaks nothing across accounts" ($leaks.Count -eq 0) "scanned=120 leaked=[$($leaks -join ',')]"

# --------------------------------------------- P20.3 unauthenticated surface
$anon = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$routes = @(
    @("GET", "/users"), @("GET", "/domains"), @("GET", "/audit"), @("GET", "/mail/messages?folder=Inbox"),
    @("GET", "/drive/tree"), @("GET", "/contacts"), @("GET", "/calendar/events"), @("GET", "/admin/health"),
    @("GET", "/fetches"), @("GET", "/tokens"), @("POST", "/mail/send"), @("GET", "/push/vapid"), @("GET", "/events")
)
$open = @()
$codes = @()
foreach ($rt in $routes) {
    $sess = @{ S = $anon; Email = "anonymous" }
    $r = Api $sess $rt[0] $rt[1] $null
    $codes += "$($rt[1])=$($r.Status)"
    if ($r.Status -notin @(401, 403)) { $open += "$($rt[1])=$($r.Status)" }
}
Check P20.3a "every protected API refuses an anonymous caller" ($open.Count -eq 0) "codes=[$($codes -join ' ')] open=[$($open -join ',')]"

$stackBase = $Base -replace "/api/v1$", ""
$stackOpen = @()
foreach ($p in @("/stack/fetch", "/stack/directory/quota/$A", "/stack/rspamd/vault")) {
    $code = 0
    try { $r = Invoke-WebRequest -Uri "$stackBase$p" -Method GET -UseBasicParsing -TimeoutSec 15; $code = [int]$r.StatusCode } catch { if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode } }
    if ($code -notin @(401, 403)) { $stackOpen += "$p=$code" }
}
Check P20.3b "internal /stack API refuses callers without the stack secret" ($stackOpen.Count -eq 0) "open=[$($stackOpen -join ',')]"

# ------------------------------------------------- P20.5 injection surfaces
$payloads = @(
    "'; DROP TABLE user; --",
    "<script>alert(1)</script>",
    "q%00null",
    "subject:%22 OR 1=1 --",
    ("a" * 500)
)
$bad = @()
foreach ($p in $payloads) {
    $enc = [uri]::EscapeDataString($p)
    $r = Api $aSess GET "/mail/search?q=$enc&folder=Inbox"
    if ($r.Status -ge 500) { $bad += "search:$($r.Status)" }
}
$vcf = "BEGIN:VCARD`r`nVERSION:3.0`r`nFN:'; DROP TABLE contact; --`r`nEMAIL;TYPE=INTERNET:x$rnd@example.com`r`nNOTE:<script>alert(1)</script>`r`nEND:VCARD`r`n"
$imp = Api $aSess POST "/contacts/import" @{ vcard = $vcf }
$list = Api $aSess GET "/contacts?page=1&limit=20"
$hostile = $false
foreach ($x in (MsgList $list)) { if ($x.name -match "DROP TABLE") { $hostile = $true } }
# the stored value must come back as inert text and the table must still exist
$users = Api $admin GET "/users?page=1&limit=5"
Check P20.5 "hostile payloads in search and vCard import neither 500 nor corrupt" ($bad.Count -eq 0 -and $imp.Status -lt 500 -and $users.Status -eq 200) "import=$($imp.Status) hostileStoredVerbatim=$hostile adminUsersAfter=$($users.Status) searchErrors=[$($bad -join ',')]"

# ---------------------------------------------------------------- cleanup
$cl = @()
if ($bFileId -gt 0) { $cl += "driveTrash=" + (Api $bSess POST "/drive/trash" @{ id = $bFileId }).Status }
if ($bContactId -gt 0) { $cl += "contact=" + (Api $bSess DELETE "/contacts/$bContactId").Status }
if ($bEventId -gt 0) { $cl += "event=" + (Api $bSess DELETE "/calendar/events/$bEventId").Status }
foreach ($u in @($A, $B)) { $cl += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cl -join ' ')"
Flush-Results "P20"
"== done =="
