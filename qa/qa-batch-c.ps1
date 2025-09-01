# qa-batch-c.ps1 — C1..C4 日历
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== C 日历 rnd=$rnd =="
$s = New-Session
$qa = "qa-cal-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $qa; password = "QaUser2026!"; display_name = "QA Cal" }
$qs = New-Session -Email $qa -Pw "QaUser2026!"
$nowU = [DateTime]::UtcNow
$from = $nowU.AddHours(-1).ToString("yyyy-MM-ddTHH:mm:ssZ")
$to = $nowU.AddDays(2).ToString("yyyy-MM-ddTHH:mm:ssZ")
$start1 = $nowU.AddHours(2).ToString("yyyy-MM-ddTHH:mm:ssZ")
$end1 = $nowU.AddHours(3).ToString("yyyy-MM-ddTHH:mm:ssZ")
$start2 = $nowU.AddHours(4).ToString("yyyy-MM-ddTHH:mm:ssZ")
$end2 = $nowU.AddHours(5).ToString("yyyy-MM-ddTHH:mm:ssZ")

# C1 建事件→列表含→改→删（含 reminder_minutes）
$sum1 = "QA-cal-$rnd 会议甲"
$c1 = Api $s POST "/calendar/events" @{ summary = $sum1; location = "QA room"; description = "c1 desc $rnd"; start = $start1; end = $end1; reminder_minutes = 10 }
$evId = $null
$evRem = -1
if ($c1.Data) { $evId = $c1.Data.id; $evRem = $c1.Data.reminder_minutes }
$sum2 = "QA-cal-$rnd 会议甲改名"
$c1up = $null; $c1rem2 = -1
if ($evId) {
    $c1up = Api $s PUT "/calendar/events/$evId" @{ summary = $sum2; start = $start2; end = $end2; reminder_minutes = 5 }
    if ($c1up.Data) { $c1rem2 = $c1up.Data.reminder_minutes }
}
$c1list = Api $s GET "/calendar/events?from=$([uri]::EscapeDataString($from))&to=$([uri]::EscapeDataString($to))"
$listHas = $false
if ($c1list.Data) { $listHas = (@($c1list.Data | Where-Object { $_.summary -eq $sum2 }).Count -ge 1) }
$c1del = $null
if ($evId) { $c1del = Api $s DELETE "/calendar/events/$evId" }
$c1after = Api $s GET "/calendar/events?from=$([uri]::EscapeDataString($from))&to=$([uri]::EscapeDataString($to))"
$listGone = $true
if ($c1after.Data) { $listGone = (@($c1after.Data | Where-Object { $_.summary -eq $sum2 }).Count -eq 0) }
Check C1 "事件 建/列/改/删 + reminder" ($c1.Status -eq 200 -and $evRem -eq 10 -and $c1up.Status -eq 200 -and $c1rem2 -eq 5 -and $listHas -and $c1del.Status -in @(200,204) -and $listGone) "c=$($c1.Status)/rem=$evRem up=$($c1up.Status)/rem=$c1rem2 list=$listHas del=$($c1del.Status) gone=$listGone"

# C2 shares 建/列/删
$sh = Api $s POST "/calendar/shares" @{ sharee_email = $qa; read_only = $true }
$shId = $null
if ($sh.Data) { $shId = $sh.Data.id }
$shl = Api $s GET "/calendar/shares"
$shHit = $false
if ($shl.Data -and $shl.Data.owned) { $shHit = (@($shl.Data.owned | Where-Object { $_.sharee_email -eq $qa }).Count -ge 1) }
$shdel = $null
if ($shId) { $shdel = Api $s DELETE "/calendar/shares/$shId" }
$shl2 = Api $s GET "/calendar/shares"
$shGone = $true
if ($shl2.Data -and $shl2.Data.owned) { $shGone = (@($shl2.Data.owned | Where-Object { $_.sharee_email -eq $qa }).Count -eq 0) }
Check C2 "calendar shares 建/列/删" ($sh.Status -eq 200 -and $shId -ne $null -and $shl.Status -eq 200 -and $shHit -and $shdel.Status -eq 204 -and $shGone) "c=$($sh.Status) id=$shId list=$shHit del=$($shdel.Status) gone=$shGone"

# 为 C3/C4 重建一个事件
$sum3 = "QA-cal-$rnd 导出校验"
$c3ev = Api $s POST "/calendar/events" @{ summary = $sum3; start = $start1; end = $end1 }

# C4 GET /calendar/feed 返回订阅 URL（含 email+token）
$feed = Api $s GET "/calendar/feed"
$feedUrl = ""
if ($feed.Data) { $feedUrl = $feed.Data.url }
$feedEmail = ""; $feedToken = ""
if ($feedUrl -match "email=([^&]+)") { $feedEmail = $Matches[1] }
if ($feedUrl -match "token=([^&]+)") { $feedToken = $Matches[1] }
Check C4 "calendar/feed 返回订阅 URL" ($feed.Status -eq 200 -and $feedUrl -match "/calendar/export.ics" -and $feedToken -ne "") "status=$($feed.Status) url=$feedUrl"

# C3 GET /calendar/export.ics（token 鉴权；无 token 应 403）
$ics = Api $s GET "/calendar/export.ics?email=$([uri]::EscapeDataString($feedEmail))&token=$([uri]::EscapeDataString($feedToken))"
$icsTxt = ""
if ($ics.Raw -and $ics.Raw.Content) { $icsTxt = [string]$ics.Raw.Content }
$icsCt = ""
try { $icsCt = [string]$ics.Raw.Headers["Content-Type"] } catch {}
$icsNoTok = Api $s GET "/calendar/export.ics?email=$([uri]::EscapeDataString($feedEmail))"
Check C3 "export.ics 200 text/calendar 含事件，无 token 403" ($ics.Status -eq 200 -and $icsCt -match "text/calendar" -and $icsTxt -match "BEGIN:VCALENDAR" -and $icsTxt -match [regex]::Escape($sum3) -and $icsNoTok.Status -eq 403) "ics=$($ics.Status) ct=$icsCt len=$($icsTxt.Length) notoken=$($icsNoTok.Status) [c3ev=$($c3ev.Status)]"

Flush-Results "C"
"== done =="
