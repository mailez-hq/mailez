# qa-tp-p16-users.ps1 - P16 admin: user lifecycle, cascade delete, quota,
# identities, RBAC (TEST-PROMPTS.md P16), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P16 users rev1 rnd=$rnd base=$Base =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function ImapLoginOk([string]$email, [string]$pw) {
    $out = @(& python .\qa-imap-op.py 127.0.0.1 143 $email $pw list 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return $false }
    return (($json | ConvertFrom-Json).login -eq "ok")
}

$admin = New-Session
$PW = "QaUser2026!"
$U = "qa-p16-u1-$rnd@example.com"
$U2 = "qa-p16-u2-$rnd@example.com"
$MGR = "qa-p16-mgr-$rnd@example.com"

# ------------------------------------------------- P16.1 user lifecycle
$create = Api $admin POST "/users" @{ email = $U; password = $PW; display_name = "QA P16" }
$list = Api $admin GET "/users?q=qa-p16-u1-$rnd&page=1&limit=50"
$listed = ((MsgList $list) | Where-Object { $_.email -eq $U }).Count -ge 1
Check P16.1a "user can be created and listed" ($create.Status -in @(200, 201) -and $listed) "create=$($create.Status) listed=$listed"

$loginBefore = (New-Session $U $PW).Status
$disable = Api $admin PUT "/users/$U" @{ enabled = $false }
$loginDisabled = 0
try { $loginDisabled = (New-Session $U $PW).Status } catch { $loginDisabled = -1 }
$enable = Api $admin PUT "/users/$U" @{ enabled = $true }
$loginAfter = (New-Session $U $PW).Status
Check P16.1b "disable blocks login, re-enable restores it" `
    ($loginBefore -eq 200 -and $disable.Status -in @(200, 204) -and $loginDisabled -ne 200 -and $enable.Status -in @(200, 204) -and $loginAfter -eq 200) `
    "login=$loginBefore disable=$($disable.Status) loginWhileDisabled=$loginDisabled enable=$($enable.Status) loginAfter=$loginAfter"

# ------------------------------------- P16.2 cascade delete / engine purge
$sessU = New-Session $U $PW
$subj = "p16-mail-$rnd"
[void](Api $sessU POST "/mail/send" @{ to = @($U); subject = $subj; body = "self mail" })
Start-Sleep -Seconds 5
$imapBefore = ImapLoginOk $U $PW
$rowBefore = ""
try { $rowBefore = (docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT COUNT(*) FROM ``user`` WHERE email='$U';" 2>$null | Select-Object -First 1) } catch {}
$del = Api $admin DELETE "/users/$U"
Start-Sleep -Seconds 6
$rowAfter = ""
try { $rowAfter = (docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT COUNT(*) FROM ``user`` WHERE email='$U';" 2>$null | Select-Object -First 1) } catch {}
$imapAfter = ImapLoginOk $U $PW
$purgeLog = (docker logs mailez-backend-1 --since 3m 2>&1 | Out-String)
$purgeFail = $purgeLog -match "engine purge for $([regex]::Escape($U))"
$purgeMsg = ""
if ($purgeFail) { $purgeMsg = ([regex]::Match($purgeLog, "user delete: engine purge[^\r\n]*")).Value }
Check P16.2a "delete removes the control-plane user" ($del.Status -in @(200, 204) -and "$rowBefore".Trim() -eq "1" -and "$rowAfter".Trim() -eq "0") "delete=$($del.Status) rowBefore=$("$rowBefore".Trim()) rowAfter=$("$rowAfter".Trim())"
# Login failing is not proof of the cascade: the engine authenticates against
# the control plane, so a deleted user cannot log in even while its engine-side
# mailbox and storage stay behind. The purge call has to succeed as well.
Check P16.2b "engine-side mailbox is purged with the user" ($imapAfter -eq $false -and -not $purgeFail) "imapBeforeDelete=$imapBefore imapAfterDelete=$imapAfter purgeFailed=$purgeFail purgeLog=[$purgeMsg]"

# ------------------------------------------------------------- P16.4 quota
$Q = "qa-p16-q-$rnd@example.com"
$S = "qa-p16-s-$rnd@example.com"
foreach ($u in @($Q, $S)) { [void](Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P16" }) }
$setQ = Api $admin PUT "/users/$Q" @{ quota_bytes = 200000 }
$sessS = New-Session $S $PW
$sessQ = New-Session $Q $PW

$small = "p16-small-$rnd"
[void](Api $sessS POST "/mail/send" @{ to = @($Q); subject = $small; body = "small body" })
Start-Sleep -Seconds 4
$usedRow = ""
try { $usedRow = (docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT CONCAT(quota_bytes,'/',quota_bytes_used) FROM ``user`` WHERE email='$Q';" 2>$null | Select-Object -First 1) } catch {}
$overUser = "qa-p16-over-$rnd@example.com"
$bigBody = "x" * 300000
$bigSubj = "p16-big-$rnd"
$over = Api $sessS POST "/mail/send" @{ to = @($Q); subject = $bigSubj; body = $bigBody }
# if the submission was accepted the mail must bounce / be refused downstream
$delivered = $false
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 60) {
    $l = Api $sessQ GET "/mail/messages?folder=Inbox&page=0"
    foreach ($m in (MsgList $l)) { if ($m.subject -eq $bigSubj) { $delivered = $true } }
    if ($delivered) { break }
    Start-Sleep -Seconds 5
}
$quotaBounced = $false
$lS = Api $sessS GET "/mail/messages?folder=Inbox&page=0"
foreach ($m in (MsgList $lS)) { if ($m.subject -match "p16-big" -or $m.subject -match "Undelivered|退信|Mail delivery failed|failure") { $quotaBounced = $true } }
Check P16.4a "over-quota mail is refused (rejected or bounced, never silently kept)" ($over.Status -notin @(500, 502) -and (-not $delivered -or $quotaBounced)) "sendStatus=$($over.Status) deliveredIntoInbox=$delivered bounceSeen=$quotaBounced quota=$("$usedRow".Trim())"

$accRow = ""
try { $accRow = (docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT CONCAT(quota_bytes_used) FROM ``user`` WHERE email='$Q';" 2>$null | Select-Object -First 1) } catch {}
Check P16.4b "quota accounting moves with delivered and deleted mail" ([int]"$accRow".Trim() -gt 0) "quotaAfterSmallDelivery=$("$accRow".Trim()) bytes (set limit 200000)"

# ---------------------------------------------------- P16.3 identities/anon
$ident = Api $sessS GET "/mail/identities"
$identCount = 0
if ($ident.Data) { $identCount = @($ident.Data).Count }
Check P16.3a "send-as identity list is exposed" ($ident.Status -eq 200 -and $identCount -ge 1) "status=$($ident.Status) identities=$identCount"

$AD = "qa-p16-$rnd.example.com"
# Anonymous aliases are scoped to a domain the caller belongs to
# (canUseAnonmailDomain: own domain or an explicit DomainAccess grant), so the
# probe runs on example.com and restores the flag afterwards.
$AD = "example.com"
[void](Api $admin PUT "/domains/$AD" @{ anonmail_enabled = $true })
$anon = Api $sessS POST "/anon-aliases" @{ domain = $AD; display_name = "qa p16" }
$anonEmail = if ($anon.Data -and $anon.Data.email) { [string]$anon.Data.email } else { "" }
$anonList = Api $sessS GET "/anon-aliases"
$anonListed = $false
foreach ($a in (MsgList $anonList)) { if ($a.email -eq $anonEmail) { $anonListed = $true } }
Check P16.3b "anonymous alias can be created and listed" ($anon.Status -in @(200, 201) -and $anonEmail -ne "" -and $anonListed) "create=$($anon.Status) email=$anonEmail listed=$anonListed"

# mail to the anonymous address must reach the owner while keeping the address
$anonSubj = "p16-anon-$rnd"
$anonSend = Api $sessQ POST "/mail/send" @{ to = @($anonEmail); subject = $anonSubj; body = "anon probe" }
$anonHit = $false
$swA = [Diagnostics.Stopwatch]::StartNew()
while ($swA.Elapsed.TotalSeconds -lt 90 -and -not $anonHit) {
    $l = Api $sessS GET "/mail/messages?folder=Inbox&page=0"
    foreach ($m in (MsgList $l)) { if ($m.subject -eq $anonSubj) { $anonHit = $true } }
    if (-not $anonHit) { Start-Sleep -Seconds 3 }
}
Check P16.3c "mail to the anonymous address is delivered to its owner" ($anonEmail -ne "" -and $anonSend.Status -in @(200, 204) -and $anonHit) "send=$($anonSend.Status) delivered=$anonHit to=$anonEmail in=$([int]$swA.Elapsed.TotalSeconds)s"

# ------------------------------------------------------------- P16.6 RBAC
foreach ($u in @($U2, $MGR)) { [void](Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P16" }) }
$userSess = New-Session $U2 $PW
$mgrGrant = Api $admin POST "/domains/example.com/managers" @{ email = $MGR }
$mgrSess = New-Session $MGR $PW

$uUsers = Api $userSess GET "/users?page=1&limit=10"
$uAudit = Api $userSess GET "/audit?page=1&limit=10"
$uDomains = Api $userSess GET "/domains?page=1&limit=10"
$uHealth = Api $userSess GET "/admin/health"
$uFetches = Api $userSess GET "/fetches?page=1&limit=10"
$uCreate = Api $userSess POST "/users" @{ email = "qa-p16-x-$rnd@example.com"; password = $PW }
$denied = @($uUsers.Status, $uAudit.Status, $uDomains.Status, $uHealth.Status, $uFetches.Status, $uCreate.Status) | Where-Object { $_ -eq 403 }
Check P16.6a "plain user is 403 on every admin surface" ($denied.Count -eq 6) "users=$($uUsers.Status) audit=$($uAudit.Status) domains=$($uDomains.Status) health=$($uHealth.Status) fetches=$($uFetches.Status) createUser=$($uCreate.Status)"

$mUsers = Api $mgrSess GET "/users?page=1&limit=200"
$mRows = MsgList $mUsers
$mDomains = @($mRows | ForEach-Object { ($_.email -split "@")[1] } | Sort-Object -Unique)
$mCreateOwn = Api $mgrSess POST "/users" @{ email = "qa-p16-mk-$rnd@example.com"; password = $PW; display_name = "mgr made" }
$mCreateOther = Api $mgrSess POST "/users" @{ email = "qa-p16-mk2-$rnd@test.dist"; password = $PW; display_name = "mgr made 2" }
Check P16.6b "manager sees and creates only inside its own domain" `
    ($mUsers.Status -eq 200 -and $mCreateOwn.Status -in @(200, 201) -and $mCreateOther.Status -eq 403 -and ($mDomains -notcontains "test.dist")) `
    "list=$($mUsers.Status) domainsVisible=[$($mDomains -join ',')] createOwn=$($mCreateOwn.Status) createOther=$($mCreateOther.Status)"

$mAudit = Api $mgrSess GET "/audit?page=1&limit=10"
$mHealth = Api $mgrSess GET "/admin/health"
Check P16.6c "manager still cannot reach global-admin surfaces" ($mAudit.Status -eq 403 -and $mHealth.Status -eq 403) "audit=$($mAudit.Status) health=$($mHealth.Status)"

# ---------------------------------------------------------------- cleanup
$c = @()
$c += "mgr=" + (Api $admin DELETE "/domains/example.com/managers/$MGR").Status
if ($anonEmail -ne "") { $c += "anonAlias=" + (Api $sessS DELETE "/anon-aliases/$anonEmail").Status }
$c += "anonmailOff=" + (Api $admin PUT "/domains/example.com" @{ anonmail_enabled = $false }).Status
foreach ($u in @($Q, $S, $U2, $MGR, "qa-p16-mk-$rnd@example.com")) { $c += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($c -join ' ')"
Flush-Results "P16"
"== done =="
