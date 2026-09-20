# qa-fix2-verify.ps1 - verification for the second fix round:
#   * /mail/send accepts the draft's "text" field for the body
#   * a domain manager can read its own domain (and only its own)
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== fix round 2 verification rnd=$rnd =="

function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-f2-a-$rnd@example.com"
$MGR = "qa-f2-mgr-$rnd@example.com"
foreach ($u in @($A, $MGR)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA fix2" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW

# ------------------------------------------------- FIX2.1 send "text" alias
$marker = "f2text$rnd"
$send = Api $aSess POST "/mail/send" @{ to = @($A); subject = "f2-text-$rnd"; text = "body via the draft field $marker" }
$found = $null
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 90 -and -not $found) {
    foreach ($m in (MsgList (Api $aSess GET "/mail/messages?folder=Inbox&page=0"))) {
        if ($m.subject -eq "f2-text-$rnd") { $found = $m }
    }
    if (-not $found) { Start-Sleep -Seconds 3 }
}
$raw = ""
if ($found) {
    $r = Api $aSess GET "/mail/raw?folder=Inbox&uid=$($found.uid)"
    if ($r.Data) { $raw = [string]$r.Data.raw }
}
$bodyOk = $raw -match [regex]::Escape($marker)
Check FIX2.1 "POST /mail/send accepts the draft's text field as the body" `
    ($send.Status -in @(200, 204) -and $bodyOk) `
    "send=$($send.Status) delivered=$($found -ne $null) bodyMarkerPresent=$bodyOk (修复前这里会发出空正文)"

# "body" keeps working
$marker2 = "f2body$rnd"
[void](Api $aSess POST "/mail/send" @{ to = @($A); subject = "f2-body-$rnd"; body = "body via the send field $marker2" })
$found2 = $null
$sw2 = [Diagnostics.Stopwatch]::StartNew()
while ($sw2.Elapsed.TotalSeconds -lt 90 -and -not $found2) {
    foreach ($m in (MsgList (Api $aSess GET "/mail/messages?folder=Inbox&page=0"))) {
        if ($m.subject -eq "f2-body-$rnd") { $found2 = $m }
    }
    if (-not $found2) { Start-Sleep -Seconds 3 }
}
$raw2 = ""
if ($found2) {
    $r2 = Api $aSess GET "/mail/raw?folder=Inbox&uid=$($found2.uid)"
    if ($r2.Data) { $raw2 = [string]$r2.Data.raw }
}
Check FIX2.2 "body field still works" ($found2 -ne $null -and ($raw2 -match [regex]::Escape($marker2))) "delivered=$($found2 -ne $null) bodyMarkerPresent=$($raw2 -match [regex]::Escape($marker2))"

# --------------------------------------------- FIX2.2 manager domain reads
$grant = Api $admin POST "/domains/example.com/managers" @{ email = $MGR }
$mgrSess = New-Session $MGR $PW
$own = Api $mgrSess GET "/domains/example.com"
$ownDkim = Api $mgrSess GET "/domains/example.com/dkim"
$list = Api $mgrSess GET "/domains?page=1&limit=200"
$listNames = @(MsgList $list | ForEach-Object { $_.name })
$ownDns = Api $mgrSess GET "/domains/example.com/dns-records"
Check FIX2.3 "manager reads its own domain (get / dkim / dns)" `
    ($grant.Status -eq 204 -and $own.Status -eq 200 -and $ownDkim.Status -eq 200 -and $ownDns.Status -eq 200) `
    "grant=$($grant.Status) get=$($own.Status) dkim=$($ownDkim.Status) dns=$($ownDns.Status)"
Check FIX2.4 "manager's domain list is scoped to its own domains" `
    ($list.Status -eq 200 -and $listNames -contains "example.com" -and ($listNames | Where-Object { $_ -ne "example.com" }).Count -eq 0) `
    "status=$($list.Status) domains=[$($listNames -join ',')] (修复前该请求直接 403)"

# other-domain reads stay closed, writes stay admin-only
[void](Api $admin POST "/domains" @{ name = "qa-f2b-$rnd.example.com" })
$other = Api $mgrSess GET "/domains/qa-f2b-$rnd.example.com"
$otherDkim = Api $mgrSess GET "/domains/qa-f2b-$rnd.example.com/dkim"
$mkDomain = Api $mgrSess POST "/domains" @{ name = "qa-f2c-$rnd.example.com" }
$delDomain = Api $mgrSess DELETE "/domains/example.com"
$genDkim = Api $mgrSess POST "/domains/example.com/dkim"
Check FIX2.5 "manager cannot read other domains or mutate any" `
    ($other.Status -eq 403 -and $otherDkim.Status -eq 403 -and $mkDomain.Status -eq 403 -and $delDomain.Status -eq 403 -and $genDkim.Status -eq 403) `
    "otherGet=$($other.Status) otherDkim=$($otherDkim.Status) createDomain=$($mkDomain.Status) deleteDomain=$($delDomain.Status) generateDkim=$($genDkim.Status)"

# admin still sees everything and can still manage
$adminList = @(MsgList (Api $admin GET "/domains?page=1&limit=200") | ForEach-Object { $_.name })
$adminCreate = Api $admin POST "/domains" @{ name = "qa-f2d-$rnd.example.com" }
$adminDelete = Api $admin DELETE "/domains/qa-f2d-$rnd.example.com"
Check FIX2.6 "admin path unchanged" `
    ($adminList.Count -ge 2 -and ($adminList -contains "example.com") -and $adminCreate.Status -in @(200, 201) -and $adminDelete.Status -in @(200, 204)) `
    "adminSeesDomains=$($adminList.Count) create=$($adminCreate.Status) delete=$($adminDelete.Status)"

$cl = @()
$cl += "mgrRevoke=" + (Api $admin DELETE "/domains/example.com/managers/$MGR").Status
$cl += "tmpDomain=" + (Api $admin DELETE "/domains/qa-f2b-$rnd.example.com").Status
foreach ($u in @($A, $MGR)) { $cl += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cl -join ' ')"
Flush-Results "FIX2"
"== done =="
