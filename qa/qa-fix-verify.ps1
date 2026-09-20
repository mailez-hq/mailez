# qa-fix-verify.ps1 - verification for the fix round.
# Covers: sort (string UID parsing), recipient-quota mapping, login rate-limit
# key fallback, contact delete 404, and the engine cascade purge using the
# shipped compose value.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== fix verification rnd=$rnd =="

function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}
function Body($r) {
    if ($r.Body) { return [string]$r.Body }
    if ($r.Raw) { return [Text.Encoding]::UTF8.GetString($r.Raw.RawContentStream.ToArray()) }
    return ""
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-fix-a-$rnd@example.com"
$B = "qa-fix-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA fix" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

# three messages so a sorted list has something to order
foreach ($n in @("alpha", "zeta", "mid")) { [void](Api $aSess POST "/mail/send" @{ to = @($B); subject = "fix-$n-$rnd"; body = "x" }) }
Start-Sleep -Seconds 6

# ------------------------------------------------------ FIX.1 non-date sorts
$plain = (MsgList (Api $bSess GET "/mail/messages?folder=Inbox&page=0")).Count
$subj = Api $bSess GET "/mail/messages?folder=Inbox&page=0&sort=subject&dir=desc"
$from = Api $bSess GET "/mail/messages?folder=Inbox&page=0&sort=from&dir=desc"
$size = Api $bSess GET "/mail/messages?folder=Inbox&page=0&sort=size&dir=desc"
$subjRows = (MsgList $subj).Count
$fromRows = (MsgList $from).Count
$sizeRows = (MsgList $size).Count
$firstSubject = (@(MsgList $subj)[0]).subject
Check FIX.1 "sort=subject/from/size return the mailbox again (was 0 rows)" `
    ($plain -ge 3 -and $subjRows -eq $plain -and $fromRows -eq $plain -and $sizeRows -eq $plain) `
    "plain=$plain subject=$subjRows from=$fromRows size=$sizeRows firstSubject=$firstSubject"

# subject sort must actually be ordered
$subjects = @(MsgList $subj | ForEach-Object { $_.subject })
$ordered = ($subjects -join "|") -eq (($subjects | Sort-Object -Descending) -join "|")
Check FIX.2 "sort=subject desc is really ordered" ($subjRows -ge 2 -and $ordered) "subjects=[$($subjects -join ' , ')] ordered=$ordered"

# ------------------------------------------------------ FIX.2 recipient quota
$Q = "qa-fix-q-$rnd@example.com"
[void](Api $admin POST "/users" @{ email = $Q; password = $PW; display_name = "QA fix quota" })
[void](Api $admin PUT "/users/$Q" @{ quota_bytes = 200000 })
[void](Api $aSess POST "/mail/send" @{ to = @($Q); subject = "fix-small-$rnd"; body = "small" })
Start-Sleep -Seconds 5
$over = Api $aSess POST "/mail/send" @{ to = @($Q); subject = "fix-big-$rnd"; body = ("x" * 300000) }
$overJson = $over.Data
if (-not $overJson) { try { $overJson = (Body $over) | ConvertFrom-Json } catch {} }
Check FIX.3 "over-quota send answers 422 with an actionable message" `
    ($over.Status -eq 422 -and $overJson.code -eq "recipient_quota_exceeded" -and "$($overJson.error)" -match "full") `
    "status=$($over.Status) code=$($overJson.code) error=$($overJson.error) body=$(Body $over)"

# ------------------------------------------------------ FIX.3 login rate key
$null = docker exec mailez-redis-1 redis-cli DEL "mailez:login:ip:" 2>&1
[void](New-Session $A $PW)
$keys = docker exec mailez-redis-1 redis-cli --scan --pattern "mailez:login:ip:*" 2>&1
$hasRealIp = @($keys | Where-Object { $_ -match "^mailez:login:ip:.+$" }).Count -ge 1
$emptyKey = @($keys | Where-Object { $_ -eq "mailez:login:ip:" }).Count -ge 1
Check FIX.4 "login limiter keys on a real address (no empty-IP bucket)" ($hasRealIp -and -not $emptyKey) "keys=[$($keys -join ',')]"

# ------------------------------------------------------ FIX.4 contact delete
$c = Api $bSess POST "/contacts" @{ name = "fix probe"; email = "fix-$rnd@example.com" }
$cid = if ($c.Data.id) { [int]$c.Data.id } else { 0 }
$foreign = Api $aSess DELETE "/contacts/$cid"
$own = Api $bSess DELETE "/contacts/$cid"
Check FIX.5 "deleting a contact you do not own answers 404 (was a silent 204)" `
    ($cid -gt 0 -and $foreign.Status -eq 404 -and $own.Status -in @(200, 204)) `
    "id=$cid foreignDelete=$($foreign.Status) ownDelete=$($own.Status)"

# ------------------------------------------------------ FIX.5 cascade purge
$P = "qa-fix-p-$rnd@example.com"
[void](Api $admin POST "/users" @{ email = $P; password = $PW; display_name = "QA fix purge" })
$pSess = New-Session $P $PW
[void](Api $pSess POST "/mail/send" @{ to = @($P); subject = "fix-purge-$rnd"; body = "purge me" })
Start-Sleep -Seconds 5
$before = docker logs mailez-backend-1 --since 1m 2>&1 | Select-String -Pattern "engine purge" | Measure-Object | Select-Object -ExpandProperty Count
[void](Api $admin DELETE "/users/$P")
Start-Sleep -Seconds 10
$backendPurge = docker logs mailez-backend-1 --since 2m 2>&1 | Select-String -Pattern "engine purge" | Measure-Object | Select-Object -ExpandProperty Count
$engineLog = (docker logs mailez-mailezine-1 --since 2m 2>&1) + (docker logs mailez-mailezine-2 --since 2m 2>&1)
$purged = ($engineLog | Select-String -Pattern "management account purge.*$([regex]::Escape($P))" | Select-Object -First 1)
$purgedLine = if ($purged) { "$($purged.Line)".Trim() } else { "" }
Check FIX.6 "user delete cascades to the engine with the shipped compose value" `
    ($backendPurge -eq $before -and $purgedLine -ne "") `
    "backendPurgeErrorsBefore=$before after=$backendPurge engineLog=[$purgedLine]"

$cl = @()
foreach ($u in @($A, $B, $Q)) { $cl += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cl -join ' ')"
Flush-Results "FIX"
"== done =="
