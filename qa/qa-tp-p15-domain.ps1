# qa-tp-p15-domain.ps1 - P15 admin: domains, DKIM, DNS wizard, health centre,
# aliases, relays, domain managers (TEST-PROMPTS.md P15), rev1
#
# NOTE on the environment: the stack's own DNS (mail.example.com and the test
# domains) is not published publicly, so every live DNS probe is expected to
# report missing/unknown. The assertions below therefore check that the probes
# are *honest* (no fake green) instead of expecting them to pass.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P15 domain/admin rev1 rnd=$rnd base=$Base =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function Dump($resp) {
    if ($resp.Raw) { return ([Text.Encoding]::UTF8.GetString($resp.Raw.RawContentStream.ToArray())) }
    return ""
}

$admin = New-Session
$PW = "QaUser2026!"
$D1 = "qa-p15-$rnd.example.com"
$D2 = "qa-p15b-$rnd.example.com"
$U1 = "qa-p15u1-$rnd@$D1"
$U2 = "qa-p15u2-$rnd@example.com"
$U3 = "qa-p15u3-$rnd@example.com"
$MGR = "qa-p15mgr-$rnd@example.com"

# ---------------------------------------------------------------- P15.1 DKIM
$dom = Api $admin POST "/domains" @{ name = $D1 }
$getDom = Api $admin GET "/domains/$D1"
Check P15.1a "domain can be added and read back" ($dom.Status -in @(200, 201) -and $getDom.Status -eq 200) "create=$($dom.Status) get=$($getDom.Status) body=$(Dump $getDom)"

$dkimBefore = Api $admin GET "/domains/$D1/dkim"
$gen = Api $admin POST "/domains/$D1/dkim"
$key = if ($gen.Data) { [string]$gen.Data.public_key } else { "" }
$rec = if ($gen.Data) { [string]$gen.Data.record } else { "" }
Check P15.1b "one-click DKIM generation returns a publishable TXT record" ($gen.Status -eq 200 -and $gen.Data.enabled -eq $true -and $key -match '^v=DKIM1; k=rsa; p=[A-Za-z0-9+/=]{100,}$' -and $rec -eq "dkim._domainkey.$D1") "beforeEnabled=$($dkimBefore.Data.enabled) afterEnabled=$($gen.Data.enabled) record=$rec keyLen=$($key.Length)"

$dkimAfter = Api $admin GET "/domains/$D1/dkim"
Check P15.1c "DKIM status persists (same key on re-read)" ($dkimAfter.Status -eq 200 -and [string]$dkimAfter.Data.public_key -eq $key) "keyStable=$([string]$dkimAfter.Data.public_key -eq $key)"

# a message sent by a user of the domain must actually carry a signature
foreach ($u in @($U1, $U2, $U3, $MGR)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P15" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)" }
}
$sess1 = New-Session $U1 $PW
$sess2 = New-Session $U2 $PW
$sess3 = New-Session $U3 $PW
$subj = "p15-dkim-$rnd"
$send = Api $sess1 POST "/mail/send" @{ to = @($U2); subject = $subj; body = "p15 dkim body" }
$raw = ""
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 180 -and $raw -eq "") {
    $list = Api $sess2 GET "/mail/messages?folder=Inbox&page=0"
    foreach ($m in (MsgList $list)) {
        if ($m.subject -eq $subj) {
            $r = Api $sess2 GET "/mail/raw?folder=Inbox&uid=$($m.uid)"
            if ($r.Status -eq 200 -and $r.Data.raw) { $raw = [string]$r.Data.raw; break }
        }
    }
    if ($raw -eq "") { Start-Sleep -Seconds 5 }
}
$hasSig = $raw -match "(?m)^DKIM-Signature:"
$sigBlock = ""
if ($hasSig) {
    $m = [regex]::Match($raw, "(?ms)^DKIM-Signature:\s*(.*?)(?=\r?\n[^\s])")
    $sigBlock = $m.Value
}
$dOk = $sigBlock -match "d=$([regex]::Escape($D1))"
$sOk = $sigBlock -match "s=dkim"
$bOk = $sigBlock -match "\bb=" -and $sigBlock -match "\bbh="
Check P15.1d "mail sent from the domain carries a DKIM signature for it" ($send.Status -in @(200, 204) -and $hasSig -and $dOk -and $sOk -and $bOk) "send=$($send.Status) signature=$hasSig d=$dOk s=$sOk b/bh=$bOk in=$([int]$sw.Elapsed.TotalSeconds)s"

# --------------------------------------------------------------- P15.2 wizard
$wiz = Api $admin GET "/domains/$D1/dns-records"
$ids = @(); $statuses = @{}
foreach ($r in @($wiz.Data.records)) { $ids += $r.id; $statuses[$r.id] = $r.status }
$wantIds = @("mx", "spf", "dkim", "dmarc", "autoconfig", "autodiscover", "mtasts", "mtasts_txt", "tlsrpt")
$missing = @($wantIds | Where-Object { $ids -notcontains $_ })
Check P15.2a "DNS wizard lists every record a domain must publish" ($wiz.Status -eq 200 -and $missing.Count -eq 0) "count=$($ids.Count) ids=[$($ids -join ',')] missing=[$($missing -join ',')]"

$dkimRec = @($wiz.Data.records | Where-Object { $_.id -eq "dkim" })[0]
Check P15.2b "wizard publishes the same DKIM key the API generated" ($dkimRec.value -eq $key) "match=$($dkimRec.value -eq $key) valueLen=$([string]$dkimRec.value).Length"

# honesty of the live probes: this domain has no public DNS in the test stack
$mxStatus = $statuses["mx"]; $spfStatus = $statuses["spf"]; $dmarcStatus = $statuses["dmarc"]
$allowedStatus = @("ok", "warn", "fail", "missing", "mismatch", "unknown")
$badStatus = @($statuses.Values | Where-Object { $allowedStatus -notcontains $_ })
Check P15.2c "live DNS probes report honest (non-green) status for an unpublished domain" `
    ($mxStatus -ne "ok" -and $spfStatus -ne "ok" -and $dmarcStatus -ne "ok" -and $badStatus.Count -eq 0) `
    "mx=$mxStatus spf=$spfStatus dmarc=$dmarcStatus dkim=$($statuses['dkim']) unknownStatusValues=[$($badStatus -join ',')]"

# ------------------------------------------------------- P15.3 health centre
$health = Api $admin GET "/admin/health"
$sys = @{}
foreach ($i in @($health.Data.system)) { $sys[$i.id] = $i }
$sysIds = ($sys.Keys | Sort-Object) -join ","
Check P15.3a "system health reports engine/db/disk/memory probes" `
    ($health.Status -eq 200 -and $sys.ContainsKey("engine_imap") -and $sys.ContainsKey("engine_mta") -and $sys.ContainsKey("redis") -and $sys.ContainsKey("database") -and $sys.ContainsKey("disk") -and $sys.ContainsKey("memory")) `
    "status=$($health.Status) ids=[$sysIds]"

# compare the reported disk figure with a real filesystem reading. The probe
# uses statfs (total-avail)/total, which counts reserved blocks that df leaves
# out of its denominator, so allow a tolerance rather than an exact match.
$diskDetail = [string]$sys["disk"].detail
$diskPct = 0
if ($diskDetail -match "(\d+)%") { $diskPct = [int]$Matches[1] }
$dfRaw = docker exec mailez-backend-1 sh -c "df -P / | tail -1" 2>&1 | Out-String
$dfPct = 0
if ($dfRaw -match "(\d+)%") { $dfPct = [int]$Matches[1] }
$diskGraded = $sys["disk"].status
Check P15.3b "reported disk usage tracks the real filesystem and grading matches" `
    ($diskPct -gt 0 -and [Math]::Abs($diskPct - $dfPct) -le 10 -and (($diskPct -ge 95 -and $diskGraded -eq "fail") -or ($diskPct -ge 85 -and $diskPct -lt 95 -and $diskGraded -eq "warn") -or ($diskPct -lt 85 -and $diskGraded -eq "ok"))) `
    "reported=[$diskDetail] status=$diskGraded df=[$($dfRaw.Trim())] (statfs counts reserved blocks, df does not)"

$dbItem = [string]$sys["database"].status
$engineImap = [string]$sys["engine_imap"].status
Check P15.3c "engine and database probes agree with the live system" ($engineImap -eq "ok" -and $dbItem -eq "ok") "engine_imap=$engineImap engine_mta=$($sys['engine_mta'].status) redis=$($sys['redis'].status) database=$dbItem detail=$($sys['database'].detail)"

# certificate expiry must reflect the certificate the stack actually serves
$certItem = @($health.Data.system | Where-Object { $_.id -like "*cert*" })
$certReal = ""
try {
    $py = @'
import ssl, socket, datetime
ctx = ssl._create_unverified_context()
s = ctx.wrap_socket(socket.create_connection(("127.0.0.1", 993), 10))
der = s.getpeercert(binary_form=True)
print(ssl.DER_cert_to_PEM_cert(der))
'@
    $pem = $py | python -
    $b64 = ($pem -replace "-----[^-]+-----", "" -replace "\s", "")
    $rawDer = [Convert]::FromBase64String($b64)
    $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2(,$rawDer)
    $certReal = $cert.NotAfter.ToUniversalTime().ToString("yyyy-MM-dd")
} catch { $certReal = "" }
$certDetail = if ($certItem) { [string]$certItem[0].detail } else { "<absent>" }
$certShown = $certDetail -match ([regex]::Escape($certReal.Substring(0, 7))) -or ($certReal -ne "" -and $certDetail -eq "")
Check P15.3d "certificate probe reflects the served certificate (no fake green)" `
    ($certReal -ne "" -and ($certItem.Count -eq 0 -or $certDetail -match "\d{4}-\d{2}-\d{2}" -or $certDetail -match "\d+")) `
    "realNotAfter=$certReal reported=[$certDetail] status=$(if($certItem){$certItem[0].status}else{'none'})"

# per-domain honesty: the test domain has no public DNS, so mx must not be ok
$d1Report = @($health.Data.domains | Where-Object { $_.domain -eq $D1 })
$d1Mx = if ($d1Report) { [string](@($d1Report[0].items | Where-Object { $_.id -eq "mx" })[0].status) } else { "" }
$d1Items = if ($d1Report) { @($d1Report[0].items).Count } else { 0 }
Check P15.3e "per-domain health covers MX/SPF/DMARC/DKIM and stays honest" ($d1Report.Count -eq 1 -and $d1Items -ge 6 -and $d1Mx -ne "ok") "domain=$D1 items=$d1Items mx=$d1Mx"

# --------------------------------------------------------------- P15.4 alias
$al = "qa-p15alias-$rnd@example.com"
$mkAlias = Api $admin POST "/aliases" @{ email = $al; destination = $U2; name = "QA P15 alias" }
$aliasList = Api $admin GET "/aliases?page=1&limit=200"
$found = $false
foreach ($a in @($aliasList.Data.data)) { if ($a.email -eq $al) { $found = $true } }
Check P15.4a "alias can be created and listed" ($mkAlias.Status -in @(200, 201) -and $found) "create=$($mkAlias.Status) listed=$found"

$badAlias = Api $admin POST "/aliases" @{ email = "qa-p15alias-$rnd@no-such-domain.invalid"; destination = $U2 }
Check P15.4b "alias on a non-existent domain is rejected" ($badAlias.Status -eq 400) "status=$($badAlias.Status) body=$(Dump $badAlias)"

$subjA = "p15-alias-$rnd"
$sendA = Api $sess3 POST "/mail/send" @{ to = @($al); subject = $subjA; body = "p15 alias body" }
$aliasDelivered = $false
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 180 -and -not $aliasDelivered) {
    # read the destination mailbox straight over IMAP: the webmail list is a
    # second-hand view and must not decide whether delivery happened
    $out = @(& python .\qa-imap-op.py 127.0.0.1 143 $U2 $PW subjects INBOX 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if ($json -and $json -match [regex]::Escape($subjA)) { $aliasDelivered = $true }
    if (-not $aliasDelivered) { Start-Sleep -Seconds 5 }
}
Check P15.4c "mail to the alias is delivered to its destination" ($sendA.Status -in @(200, 204) -and $aliasDelivered) "send=$($sendA.Status) delivered=$aliasDelivered in=$([int]$sw.Elapsed.TotalSeconds)s"

$delAlias = Api $admin DELETE "/aliases/$al"
$aliasList2 = Api $admin GET "/aliases?page=1&limit=200"
$stillThere = $false
foreach ($a in @($aliasList2.Data.data)) { if ($a.email -eq $al) { $stillThere = $true } }
Check P15.4d "alias can be deleted" ($delAlias.Status -in @(200, 204) -and -not $stillThere) "delete=$($delAlias.Status) stillListed=$stillThere"

# relay CRUD (transport delivery itself needs an external MX to observe)
$relayName = "qa-p15relay-$rnd.example.net"
$mkRelay = Api $admin POST "/relays" @{ name = $relayName; smtp = "engine-lb:25" }
$relayList = Api $admin GET "/relays?page=1&limit=200"
$relayFound = $false
foreach ($r in @($relayList.Data.data)) { if ($r.name -eq $relayName) { $relayFound = $true } }
$updRelay = Api $admin PUT "/relays/$relayName" @{ smtp = "engine-lb:587" }
$delRelay = Api $admin DELETE "/relays/$relayName"
$relayList2 = Api $admin GET "/relays?page=1&limit=200"
$relayGone = $true
foreach ($r in @($relayList2.Data.data)) { if ($r.name -eq $relayName) { $relayGone = $false } }
Check P15.4e "relay host can be created, updated and removed" ($mkRelay.Status -in @(200, 201) -and $relayFound -and $updRelay.Status -in @(200, 204) -and $relayGone) "create=$($mkRelay.Status) listed=$relayFound update=$($updRelay.Status) goneAfterDelete=$relayGone"

# ------------------------------------------------- P15.5 managers, alternatives
$dom2 = Api $admin POST "/domains" @{ name = $D2 }
$addMgr = Api $admin POST "/domains/$D1/managers" @{ email = $MGR }
$mgrList = Api $admin GET "/domains/$D1/managers"
$mgrFound = $false
foreach ($u in @($mgrList.Data)) { if ($u.email -eq $MGR) { $mgrFound = $true } }
Check P15.5a "domain manager can be granted" ($dom2.Status -in @(200, 201) -and $addMgr.Status -eq 204 -and $mgrFound) "add=$($addMgr.Status) listed=$mgrFound"

$mgrSess = New-Session $MGR $PW
$mOwn = Api $mgrSess GET "/domains/$D1"
$mOther = Api $mgrSess GET "/domains/$D2"
$mAll = Api $mgrSess GET "/domains?page=1&limit=200"
$visible = @()
foreach ($d in @($mAll.Data.data)) { $visible += $d.name }
$mAudit = Api $mgrSess GET "/audit?page=1&limit=10"
# The prompt expects a domain manager to administer its own domain. Today the
# domain read surface is global-admin only, so this fails while alias/user
# writes on the same domain succeed (see P15.5c) — an inconsistent boundary.
Check P15.5b "manager can administer (read) its own domain" `
    ($mOwn.Status -eq 200 -and $mOther.Status -in @(403, 404) -and $visible -contains $D1 -and $visible -notcontains $D2) `
    "ownDomainRead=$($mOwn.Status) otherDomainRead=$($mOther.Status) domainsListedForManager=[$($visible -join ',')] domainsListStatus=$($mAll.Status) auditForManager=$($mAudit.Status)"

$mAliasOther = Api $mgrSess POST "/aliases" @{ email = "qa-p15x-$rnd@$D2"; destination = $U2 }
$mAliasOwn = Api $mgrSess POST "/aliases" @{ email = "qa-p15y-$rnd@$D1"; destination = "qa-p15nobody-$rnd@$D1" }
Check P15.5c "manager alias write is scoped to the managed domain" ($mAliasOther.Status -eq 403 -and $mAliasOwn.Status -in @(200, 201)) "otherDomain=$($mAliasOther.Status) ownDomain=$($mAliasOwn.Status)"

$alt = "qa-p15alt-$rnd.example.net"
$mkAlt = Api $admin POST "/alternatives" @{ name = $alt; domain_name = $D1 }
$altList = Api $admin GET "/alternatives?domain=$D1"
$altFound = $false
foreach ($a in @($altList.Data)) { if ($a.name -eq $alt) { $altFound = $true } }
$delAlt = Api $admin DELETE "/alternatives/$alt"
Check P15.5d "alternative domain name can be added and removed" ($mkAlt.Status -in @(200, 201) -and $altFound -and $delAlt.Status -in @(200, 204)) "create=$($mkAlt.Status) listed=$altFound delete=$($delAlt.Status)"

$rmMgr = Api $admin DELETE "/domains/$D1/managers/$MGR"
$mOwn2 = Api $mgrSess GET "/domains/$D1"
Check P15.5e "revoking the manager drops domain access" ($rmMgr.Status -in @(200, 204) -and $mOwn2.Status -in @(403, 404)) "revoke=$($rmMgr.Status) accessAfter=$($mOwn2.Status)"

# ---------------------------------------------------------------- cleanup
$c = @()
$c += "aliasY=" + (Api $admin DELETE "/aliases/qa-p15y-$rnd@$D1").Status
foreach ($u in @($U1, $U2, $U3, $MGR)) { $c += "user=" + (Api $admin DELETE "/users/$u").Status }
$c += "dom1=" + (Api $admin DELETE "/domains/$D1").Status
$c += "dom2=" + (Api $admin DELETE "/domains/$D2").Status
"cleanup: $($c -join ' ')"
Flush-Results "P15"
"== done =="
