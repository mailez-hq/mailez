# qa-batch-h.ps1 — H1..H4 跨域回归
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== H 跨域与回归 rnd=$rnd =="
# 与 qa-lib 相同的 Base 解析（QA_BASE 优先）
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$s = New-Session

# H1 全新用户端到端：建户→发信→读信
$h1 = "qa-h1-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $h1; password = "QaUser2026!"; display_name = "QA H1" }
$h1subj = "QA-h1-$rnd 端到端"
$h1send = Api $s POST "/mail/send" @{ to = @($h1); subject = $h1subj; body = "h1 body $rnd" }
Start-Sleep -Seconds 6
$h1s = New-Session -Email $h1 -Pw "QaUser2026!"
$h1list = Api $h1s GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$h1msg = @($h1list.Data | Where-Object { $_.subject -eq $h1subj }) | Select-Object -First 1
$h1read = $null
$h1readOk = $false
if ($h1msg) {
    $h1read = Api $h1s GET "/mail/message?folder=Inbox&uid=$($h1msg.uid)"
    $h1readOk = ($h1read.Status -eq 200 -and $h1read.Data.subject -eq $h1subj)
}
Check H1 "全新用户 建/发/收/读 全链路" ($h1send.Status -in @(200,201,204) -and $h1s.Status -eq 200 -and $h1list.Status -eq 200 -and $h1msg -ne $null -and $h1readOk) "send=$($h1send.Status) login=$($h1s.Status) list=$($h1list.Status) found=$($h1msg -ne $null) read=$($h1read.Status)"

# H2 权限隔离：A 读不到 B 的邮件/云盘
$aU = "qa-h2a-$rnd@example.com"
$bU = "qa-h2b-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $aU; password = "QaUser2026!"; display_name = "QA H2A" }
$null = Api $s POST "/users" @{ email = $bU; password = "QaUser2026!"; display_name = "QA H2B" }
$bS = New-Session -Email $bU -Pw "QaUser2026!"
$aS = New-Session -Email $aU -Pw "QaUser2026!"
$bSubj = "QA-h2-$rnd B的秘密"
$null = Api $bS POST "/mail/send" @{ to = @($bU); subject = $bSubj; body = "only for B" }
Start-Sleep -Seconds 6
# 邮件隔离：A 的收件箱不出现 B 的主题
$aIn = Api $aS GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$aNoLeak = $true
if ($aIn.Data) { $aNoLeak = (@($aIn.Data | Where-Object { $_.subject -eq $bSubj }).Count -eq 0) }
# 云盘隔离：B 上传文件，A 直接下载 403；A 的树里没有 B 的文件
$bContent = [Text.Encoding]::UTF8.GetBytes("b secret $rnd")
$boundary = "----QABoundary" + [guid]::NewGuid().ToString("N")
$ms = New-Object System.IO.MemoryStream
$w = New-Object System.IO.BinaryWriter($ms)
$enc8 = [Text.Encoding]::UTF8
$w.Write($enc8.GetBytes("--$boundary`r`n"))
$w.Write($enc8.GetBytes("Content-Disposition: form-data; name=`"parent_id`"`r`n`r`n0`r`n"))
$w.Write($enc8.GetBytes("--$boundary`r`n"))
$w.Write($enc8.GetBytes("Content-Disposition: form-data; name=`"file`"; filename=`"qa-h2-$rnd.txt`"`r`n"))
$w.Write($enc8.GetBytes("Content-Type: application/octet-stream`r`n`r`n"))
$w.Write($bContent)
$w.Write($enc8.GetBytes("`r`n--$boundary--`r`n"))
$w.Flush()
$bUpRaw = $null
try {
    $bUpRaw = Invoke-WebRequest -Uri "$Base/drive/upload" -Method POST -Body $ms.ToArray() -ContentType "multipart/form-data; boundary=$boundary" -WebSession $bS.S -UseBasicParsing -TimeoutSec 60
} catch {}
$ms.Close(); $w.Close()
$bFileId = $null
if ($bUpRaw) {
    try { $bFileId = ([Text.Encoding]::UTF8.GetString($bUpRaw.RawContentStream.ToArray()) | ConvertFrom-Json).id } catch {}
}
$aDl = $null
if ($bFileId) { $aDl = Api $aS GET "/drive/download/$bFileId" }
$aShare = $null
if ($bFileId) { $aShare = Api $aS GET "/drive/share/$bFileId" }
$aTree = Api $aS GET "/drive/tree?parent_id=0"
$aTreeNoB = $true
if ($aTree.Data) { $aTreeNoB = (@($aTree.Data | Where-Object { "$($_.name)" -like "qa-h2-$rnd*" }).Count -eq 0) }
Check H2 "A 读不到 B 的邮件/云盘(403/404/空)" ($aIn.Status -eq 200 -and $aNoLeak -and $bFileId -ne $null -and $aDl.Status -eq 403 -and $aShare.Status -eq 404 -and $aTreeNoB) "mail=$($aIn.Status)/leak=$(-not $aNoLeak) bFile=$bFileId dl=$($aDl.Status) share=$($aShare.Status) treeNoB=$aTreeNoB"

# H3 回归：admin /mail/messages 200（历史 502 已修复）
$h3 = Api $s GET "/mail/messages?folder=INBOX&page=0&sort=date&dir=desc"
$h3n = -1
if ($h3.Data) { $h3n = @($h3.Data).Count }
Check H3 "admin /mail/messages 200" ($h3.Status -eq 200) "status=$($h3.Status) items=$h3n err=$($h3.Err)"

# H4 页面 SSR 抽查：webmail 与 admin 均应 200 + HTML 标记
# 端口可用环境变量覆盖（默认 dev 档 3001/3000；compose 档传 8083/18082）
$wmUrl = if ($env:QA_WEBMAIL_URL) { $env:QA_WEBMAIL_URL } else { "http://127.0.0.1:3001/" }
$adUrl = if ($env:QA_ADMIN_URL) { $env:QA_ADMIN_URL } else { "http://127.0.0.1:3000/" }
$wmStatus = 0; $wmHtml = $false
try {
    $wm = Invoke-WebRequest -Uri $wmUrl -Method GET -UseBasicParsing -TimeoutSec 60
    $wmStatus = [int]$wm.StatusCode
    $wmHtml = ([string]$wm.Content -match "<html" -or [string]$wm.Content -match "text/html")
} catch {
    if ($_.Exception.Response) { $wmStatus = [int]$_.Exception.Response.StatusCode }
}
$adStatus = 0; $adHtml = $false
try {
    $ad = Invoke-WebRequest -Uri $adUrl -Method GET -UseBasicParsing -TimeoutSec 60
    $adStatus = [int]$ad.StatusCode
    $adHtml = ([string]$ad.Content -match "<html" -or [string]$ad.Content -match "text/html")
} catch {
    if ($_.Exception.Response) { $adStatus = [int]$_.Exception.Response.StatusCode }
}
Check H4 "页面 SSR webmail/admin 均 200 HTML" ($wmStatus -eq 200 -and $wmHtml -and $adStatus -eq 200 -and $adHtml) "webmail=$wmStatus/$wmHtml admin=$adStatus/$adHtml"

Flush-Results "H"
"== done =="
