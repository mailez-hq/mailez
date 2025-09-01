# qa-batch-g.ps1 — G1..G4 管理端
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== G 管理端 rnd=$rnd =="
# 与 qa-lib 相同的 Base 解析（QA_BASE 优先），供裸登录请求使用
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$s = New-Session
$qa = "qa-admin-$rnd@example.com"

# G1 /admin/overview 含统计
$ov = Api $s GET "/admin/overview"
$ovUsers = -1; $ovDriveFiles = -1; $ovDomain = ""
if ($ov.Data) {
    if ($ov.Data.users -ne $null) { $ovUsers = [long]$ov.Data.users }
    if ($ov.Data.drive_files -ne $null) { $ovDriveFiles = [long]$ov.Data.drive_files }
    $ovDomain = "$($ov.Data.domain)"
}
Check G1 "admin/overview 含统计" ($ov.Status -eq 200 -and $ovUsers -ge 0 -and $ovDriveFiles -ge 0) "status=$($ov.Status) users=$ovUsers drive_files=$ovDriveFiles domain=$ovDomain"

# G2 用户创建/列表/停用/启用
$g2create = Api $s POST "/users" @{ email = $qa; password = "QaUser2026!"; display_name = "QA Admin" }
$g2list = Api $s GET "/users?page=1&limit=50&q=$([uri]::EscapeDataString("qa-admin-$rnd"))"
$g2n = 0
if ($g2list.Data -and $g2list.Data.data) { $g2n = @($g2list.Data.data).Count }
$g2off = Api $s PUT "/users/$qa" @{ enabled = $false }
$g2loginOff = 0
try {
    $body = @{ email = $qa; pw = "QaUser2026!" } | ConvertTo-Json -Compress
    $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($body)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40
} catch {
    if ($_.Exception.Response) { $g2loginOff = [int]$_.Exception.Response.StatusCode }
}
$g2on = Api $s PUT "/users/$qa" @{ enabled = $true }
$g2qs = New-Session -Email $qa -Pw "QaUser2026!"
$g2loginOn = $g2qs.Status
Check G2 "用户 建/列/停用/启用" ($g2create.Status -in @(200,201) -and $g2list.Status -eq 200 -and $g2n -ge 1 -and $g2off.Status -eq 200 -and $g2loginOff -eq 401 -and $g2on.Status -eq 200 -and $g2loginOn -eq 200) "c=$($g2create.Status) list=$($g2list.Status)/$g2n off=$($g2off.Status) loginOff=$g2loginOff on=$($g2on.Status) loginOn=$g2loginOn"

# G3 审计日志有记录
$au = Api $s GET "/audit?page=1&limit=10"
$auTotal = -1; $auRows = 0
if ($au.Data) {
    if ($au.Data.total -ne $null) { $auTotal = [long]$au.Data.total }
    if ($au.Data.data) { $auRows = @($au.Data.data).Count }
}
Check G3 "审计日志有记录" ($au.Status -eq 200 -and $auTotal -ge 1 -and $auRows -ge 1) "status=$($au.Status) total=$auTotal rows=$auRows"

# G4 公告 发布/读取/撤下（复用 G2 已启用的 qa 用户会话）
$qs = $g2qs
$annSubj = "QA公告$rnd"
$annPut = Api $s PUT "/announcement" @{ subject = $annSubj; body = "公告内容 $rnd" }
$annRead = Api $s GET "/announcement"
$annHit = $false
if ($annRead.Data) { $annHit = ("$($annRead.Data.subject)" -eq $annSubj) }
$qaRead = Api $qs GET "/announcement"
$qaHit = $false
if ($qaRead.Data) { $qaHit = ("$($qaRead.Data.subject)" -eq $annSubj) }
$annDel = Api $s DELETE "/announcement"
$annGone = Api $qs GET "/announcement"
$goneOk = ($annGone.Status -eq 204 -or $annGone.Data -eq $null)
Check G4 "公告 发布/读取/撤下" ($annPut.Status -eq 200 -and $annRead.Status -eq 200 -and $annHit -and $qaRead.Status -eq 200 -and $qaHit -and $annDel.Status -in @(200,204) -and $goneOk) "put=$($annPut.Status) read=$($annRead.Status)/$annHit qaRead=$($qaRead.Status)/$qaHit del=$($annDel.Status) gone=$($annGone.Status)"

Flush-Results "G"
"== done =="
