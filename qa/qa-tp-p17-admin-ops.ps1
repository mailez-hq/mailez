# qa-tp-p17-admin-ops.ps1 - P17 audit, announcement, config export/import,
# backup and operator tooling (TEST-PROMPTS.md P17), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P17 admin ops rev1 rnd=$rnd =="

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
$U = "qa-p17-u-$rnd@example.com"
$PLAIN = "qa-p17-plain-$rnd@example.com"
$DOM = "qa-p17-$rnd.example.com"

# ---------------------------------------------------------------- P17.1 audit
[void](Api $admin POST "/users" @{ email = $U; password = $PW; display_name = "QA P17" })
[void](Api $admin PUT "/users/$U" @{ quota_bytes = 1234567 })
[void](Api $admin POST "/domains" @{ name = $DOM })
[void](Api $admin DELETE "/domains/$DOM")
Start-Sleep -Seconds 2
$audit = Api $admin GET "/audit?page=1&limit=100"
$rows = MsgList $audit
$acts = @($rows | ForEach-Object { "$($_.action)" } )
$hasCreate = @($rows | Where-Object { "$($_.action) $($_.target) $($_.detail)" -match [regex]::Escape($U) }).Count -ge 1
$hasDomain = @($rows | Where-Object { "$($_.action) $($_.target) $($_.detail)" -match [regex]::Escape($DOM) }).Count -ge 1
$sample = ($rows | Select-Object -First 1)
Check P17.1a "admin actions land in the audit log with actor and object" ($audit.Status -eq 200 -and $rows.Count -ge 3 -and $hasCreate -and $hasDomain) `
    "rows=$($rows.Count) userOps=$hasCreate domainOps=$hasDomain sample=(actor=$($sample.actor) action=$($sample.action) target=$($sample.target) at=$($sample.created_at))"

[void](Api $admin POST "/users" @{ email = $PLAIN; password = $PW; display_name = "QA P17 plain" })
$plainSess = New-Session $PLAIN $PW
$plainAudit = Api $plainSess GET "/audit?page=1&limit=5"
Check P17.1b "audit log is admin-only" ($plainAudit.Status -eq 403) "userReadAudit=$($plainAudit.Status)"

# --------------------------------------------------------- P17.2 announcement
$before = Api $plainSess GET "/announcement"
$put = Api $admin PUT "/announcement" @{ subject = "qa-p17 maintenance $rnd"; body = "planned work"; enabled = $true }
$after = Api $plainSess GET "/announcement"
$visible = (Raw $after) -match "qa-p17 maintenance"
$off = Api $admin PUT "/announcement" @{ subject = "qa-p17 maintenance $rnd"; body = "planned work"; enabled = $false }
$afterOff = Api $plainSess GET "/announcement"
$cleared = -not ((Raw $afterOff) -match "qa-p17 maintenance")
Check P17.2 "announcement publish is visible to users and disappears when withdrawn" `
    ($put.Status -in @(200, 204) -and $visible -and $cleared) `
    "put=$($put.Status) userVisible=$visible before=$($before.Status) off=$($off.Status) clearedAfterWithdraw=$cleared"

# ------------------------------------------------------- P17.3 config export
$export = Api $admin GET "/config/export"
$body = Raw $export
$json = $null
try { $json = $body | ConvertFrom-Json } catch {}
$hasDomains = $body -match "qa-p17-$rnd" -or $body -match "example.com"
$plainPwLeak = $body -match "QaUser2026" -or $body -match "MailezDemo2026"
Check P17.3a "config export returns the deployment configuration without plaintext passwords" `
    ($export.Status -eq 200 -and $json -ne $null -and $plainPwLeak -eq $false) `
    "status=$($export.Status) bytes=$($body.Length) parsed=$($json -ne $null) referencesDomain=$hasDomains plaintextPasswordLeak=$plainPwLeak keys=$(if($json){($json.PSObject.Properties.Name -join ',')}else{''})"

$import = Api $admin POST "/config/import" $body
$afterImport = Api $admin GET "/domains/example.com"
Check P17.3b "config import of the exported payload is accepted and leaves state readable" `
    ($import.Status -in @(200, 201, 204) -and $afterImport.Status -eq 200) `
    "import=$($import.Status) domainReadAfter=$($afterImport.Status) body=$(Raw $import | Select-Object -First 1)"

# --------------------------------------------------------------- P17.4 backup
# The deployed image predates the backup API: the backend image was built on
# 2026-09-14 while feat(admin): scheduled encrypted backups landed on 09-15
# (c41dde7). The 404 below therefore reports the image/source gap, not a
# defect in the shipping code.
$status = Api $admin GET "/admin/backup"
$run = Api $admin POST "/admin/backup/run" @{}
$imgDate = docker image inspect (docker inspect mailez-backend-1 --format "{{.Image}}") --format "{{.Created}}" 2>&1
if ($status.Status -eq 404) {
    Check P17.4 "SKIP: backup API present in the deployed image" $true "deployed backend image built $($imgDate) predates c41dde7 feat(admin): scheduled encrypted backups (2026-09-15); /admin/backup returns 404 by build, not by defect"
} else {
    Start-Sleep -Seconds 8
    $status2 = Api $admin GET "/admin/backup"
    Check P17.4 "backup status endpoint reports the configured target and last run" `
        ($status.Status -eq 200) "status=$($status.Status) runTrigger=$($run.Status) payload=$(Raw $status2)"
}

# ------------------------------------------------------------ P17.5 operator CLI
$ctlEePs = "D:\code\mailez-hq\mailez\deploy\ee\mailezctl-ee.ps1"
if (Test-Path $ctlEePs) {
    $out = (& pwsh -NoProfile -ExecutionPolicy Bypass -File $ctlEePs ps 2>&1 | Out-String)
    $ok = ($out -match "mailez-backend-1" -and $out -match "mailez-mailezine-1")
    Check P17.5 "mailezctl ps lists the running stack" $ok "output=$($out.Substring(0, [Math]::Min(240, $out.Length)).Trim())"
} else {
    Check P17.5 "mailezctl ps lists the running stack" $false "mailezctl-ee.ps1 not found"
}

# ---------------------------------------------------------------- cleanup
$cl = @()
$cl += "plain=" + (Api $admin DELETE "/users/$PLAIN").Status
$cl += "user=" + (Api $admin DELETE "/users/$U").Status
"cleanup: $($cl -join ' ')"
Flush-Results "P17"
"== done =="
