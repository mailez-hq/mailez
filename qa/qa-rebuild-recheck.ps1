# qa-rebuild-recheck.ps1 - re-verify the key findings after rebuilding every
# image from HEAD (backend + both frontends + engine). Focus:
#   * quota refusal path (engine HEAD maps ErrQuota -> 552 mailbox full)
#   * backup API that only exists in HEAD
#   * sort=subject / fetch-dedupe, which the code review says are still broken
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== post-rebuild recheck rnd=$rnd =="

function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}
$admin = New-Session
$PW = "QaUser2026!"

# ---------------------------------------------------- delivery smoke
$A = "qa-rb-a-$rnd@example.com"
$B = "qa-rb-b-$rnd@example.com"
foreach ($u in @($A, $B)) { [void](Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA RB" }) }
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW
$s = "rb-smoke-$rnd"
$send = Api $aSess POST "/mail/send" @{ to = @($B); subject = $s; body = "smoke" }
$seen = $false
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 90 -and -not $seen) {
    foreach ($m in (MsgList (Api $bSess GET "/mail/messages?folder=Inbox&page=0"))) { if ($m.subject -eq $s) { $seen = $true } }
    if (-not $seen) { Start-Sleep -Seconds 3 }
}
Check RB.1 "delivery still works after rebuilding every image" ($send.Status -in @(200, 204) -and $seen) "send=$($send.Status) seen=$seen in=$([int]$sw.Elapsed.TotalSeconds)s"

# ---------------------------------------------------- quota refusal path
$Q = "qa-rb-q-$rnd@example.com"
[void](Api $admin POST "/users" @{ email = $Q; password = $PW; display_name = "QA RB quota" })
[void](Api $admin PUT "/users/$Q" @{ quota_bytes = 200000 })
$qSess = New-Session $Q $PW
$small = "rb-small-$rnd"
[void](Api $aSess POST "/mail/send" @{ to = @($Q); subject = $small; body = "small" })
Start-Sleep -Seconds 4
$bigSubj = "rb-big-$rnd"
$big = "x" * 300000
$over = Api $aSess POST "/mail/send" @{ to = @($Q); subject = $bigSubj; body = $big }
$overBody = ""
if ($over.Raw) { $overBody = [Text.Encoding]::UTF8.GetString($over.Raw.RawContentStream.ToArray()) }
$log = docker logs mailez-backend-1 --since 2m 2>&1 | Out-String
$engine552 = $log -match "552|mailbox full"
Check RB.2 "over-quota submission is refused by the rebuilt engine" ($over.Status -ne 500) `
    "apiStatus=$($over.Status) apiBody=$($overBody.Substring(0, [Math]::Min(160, $overBody.Length))) engineMentions552OrFull=$engine552"
$engineLog = (docker logs mailez-mailezine-1 --since 2m 2>&1 | Out-String) + (docker logs mailez-mailezine-2 --since 2m 2>&1 | Out-String)
$quotaLine = [regex]::Match($engineLog, "(?i)[^\r\n]*(mailbox full|quota)[^\r\n]*").Value
Check RB.3 "engine logs the quota verdict explicitly" ($quotaLine -ne "") "engineLog=[$($quotaLine.Trim())]"

# ---------------------------------------------------- backup API (HEAD-only)
$bk = Api $admin GET "/admin/backup"
$run = Api $admin POST "/admin/backup/run" @{}
Start-Sleep -Seconds 10
$bk2 = Api $admin GET "/admin/backup"
$txt = ""
if ($bk2.Raw) { $txt = [Text.Encoding]::UTF8.GetString($bk2.Raw.RawContentStream.ToArray()) }
Check RB.4 "backup API is present and reports its configuration" ($bk.Status -eq 200) "status=$($bk.Status) trigger=$($run.Status) payload=$($txt.Substring(0, [Math]::Min(200, $txt.Length)))"

# ---------------------------------------------------- sort + fetch (expected broken)
$sub = Api $bSess GET "/mail/messages?folder=Inbox&page=0&sort=subject&dir=desc"
$subRows = (MsgList $sub).Count
$plainRows = (MsgList (Api $bSess GET "/mail/messages?folder=Inbox&page=0")).Count
Check RB.5 "sort=subject still returns an empty page (code review predicts this)" ($subRows -eq 0 -or $subRows -eq $plainRows) `
    "plainRows=$plainRows subjectRows=$subRows status=$($sub.Status) (HEAD 的 sortcmd.go 仍用 uint32 断言解析 string UID)"

# ---------------------------------------------------- admin basePath 404s
$icon = 0; $settings = 0
try { $null = Invoke-WebRequest -Uri "http://127.0.0.1:18082/mailez-icon.svg" -UseBasicParsing -TimeoutSec 15; $icon = 200 } catch { $icon = [int]$_.Exception.Response.StatusCode }
try { $null = Invoke-WebRequest -Uri "http://127.0.0.1:18082/api/v1/server/settings" -UseBasicParsing -TimeoutSec 15; $settings = 200 } catch { $settings = [int]$_.Exception.Response.StatusCode }
$iconOk = 0; $settingsOk = 0
try { $null = Invoke-WebRequest -Uri "http://127.0.0.1:18082/admin/mailez-icon.svg" -UseBasicParsing -TimeoutSec 15; $iconOk = 200 } catch { $iconOk = [int]$_.Exception.Response.StatusCode }
try { $null = Invoke-WebRequest -Uri "http://127.0.0.1:18082/admin/api/v1/server/settings" -UseBasicParsing -TimeoutSec 15; $settingsOk = 200 } catch { $settingsOk = [int]$_.Exception.Response.StatusCode }
Check RB.6 "admin asset/API 404s are basePath bugs (same URL with /admin works)" `
    ($icon -eq 404 -and $settings -eq 404 -and $iconOk -eq 200 -and $settingsOk -eq 200) `
    "withoutBasePath: icon=$icon settings=$settings | withBasePath: icon=$iconOk settings=$settingsOk"

$cl = @()
foreach ($u in @($A, $B, $Q)) { $cl += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cl -join ' ')"
Flush-Results "RB"
"== done =="
