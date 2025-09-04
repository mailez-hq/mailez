# qa-batch-d.ps1 — D1..D5 联系人
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== D 联系人 rnd=$rnd =="
$s = New-Session
$qa = "qa-contact-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $qa; password = "QaUser2026!"; display_name = "QA Contact" }
$qs = New-Session -Email $qa -Pw "QaUser2026!"

# D1 建→列→改→删
$cName = "QA联系$rnd"
$c1 = Api $qs POST "/contacts" @{ name = $cName; email = "qa-d1-$rnd@example.com"; comment = "d1 note"; groups = "qa组" }
$cId = $null
if ($c1.Data) { $cId = $c1.Data.id }
$cList = Api $qs GET "/contacts"
$cHit = $false
if ($cList.Data) { $cHit = (@($cList.Data | Where-Object { $_.email -eq "qa-d1-$rnd@example.com" }).Count -ge 1) }
$cUpName = "QA联系$rnd-改"
$cUp = $null
if ($cId) { $cUp = Api $qs PUT "/contacts/$cId" @{ name = $cUpName } }
$cDel = $null
if ($cId) { $cDel = Api $qs DELETE "/contacts/$cId" }
Check D1 "联系人 建/列/改/删" ($c1.Status -eq 201 -and $cHit -and $cUp.Status -eq 200 -and $cDel.Status -eq 204) "c=$($c1.Status) list=$cHit up=$($cUp.Status) del=$($cDel.Status)"

# D2 导入→导出 往返一致
$vcf = "BEGIN:VCARD`r`nVERSION:3.0`r`nFN:QA D2 One`r`nN:One;QA;;;`r`nEMAIL:qa-d2-$rnd-a@example.com`r`nEND:VCARD`r`nBEGIN:VCARD`r`nVERSION:3.0`r`nFN:QA D2 Two`r`nN:Two;QA;;;`r`nEMAIL:qa-d2-$rnd-b@example.com`r`nEND:VCARD`r`n"
$imp = Api $qs POST "/contacts/import" @{ data = $vcf }
$impAdded = -1
if ($imp.Data) { $impAdded = $imp.Data.added }
$exp = Api $qs GET "/contacts/export"
$expTxt = ""
if ($exp.Raw -and $exp.Raw.Content) { $expTxt = [string]$exp.Raw.Content }
$expCt = ""
try { $expCt = [string]$exp.Raw.Headers["Content-Type"] } catch {}
$roundTrip = ($expTxt -match [regex]::Escape("qa-d2-$rnd-a@example.com") -and $expTxt -match [regex]::Escape("qa-d2-$rnd-b@example.com"))
$imp2 = Api $qs POST "/contacts/import" @{ data = $vcf }
$imp2Added = -1
if ($imp2.Data) { $imp2Added = $imp2.Data.added }
Check D2 "导入→导出往返一致（重复导入 added=0）" ($imp.Status -eq 200 -and $impAdded -ge 2 -and $exp.Status -eq 200 -and $expCt -match "vcard" -and $roundTrip -and $imp2Added -eq 0) "imp=$($imp.Status)/added=$impAdded exp=$($exp.Status) ct=$expCt round=$roundTrip reimp=$($imp2.Status)/added=$imp2Added"

# D3 dedupe 同邮箱合并
$dupE = "qa-dup-$rnd@example.com"
$null = Api $qs POST "/contacts" @{ name = "短名"; email = $dupE }
$null = Api $qs POST "/contacts" @{ name = "这是一个更长的名字$rnd"; email = $dupE; comment = "richer" }
$dd = Api $qs POST "/contacts/dedupe" @{}
$ddMerged = -1
if ($dd.Data) { $ddMerged = $dd.Data.merged }
$dAfter = Api $qs GET "/contacts"
$dupLeft = @($dAfter.Data | Where-Object { $_.email -eq $dupE }).Count
Check D3 "dedupe 合并同邮箱" ($dd.Status -eq 200 -and $ddMerged -ge 1 -and $dupLeft -eq 1) "dd=$($dd.Status) merged=$ddMerged left=$dupLeft"

# D4 org 联系人（只读组织通讯录，CE/空库允许为空）
$org = Api $qs GET "/contacts/org"
$orgN = 0
if ($org.Data) { $orgN = @($org.Data).Count }
Check D4 "org 组织通讯录 200" ($org.Status -eq 200) "status=$($org.Status) rows=$orgN"

# D5 CardDAV get/set/sync（url 指向后端容器自身回环公开端点，sync 应 200）
$cdGet0 = Api $qs GET "/contacts/carddav"
$davUrl = "http://127.0.0.1:8080/api/v1/server/settings"
$cdSet = Api $qs PUT "/contacts/carddav" @{ url = $davUrl; username = $qa; password = "QaUser2026!" }
$cdGet1 = Api $qs GET "/contacts/carddav"
$g1Url = ""; $g1Auth = $false
if ($cdGet1.Data) { $g1Url = $cdGet1.Data.url; $g1Auth = $cdGet1.Data.has_auth }
$cdSync = Api $qs POST "/contacts/carddav/sync" @{}
$cdClr = Api $qs PUT "/contacts/carddav" @{ url = "" }
$cdGet2 = Api $qs GET "/contacts/carddav"
$g2Url = "x"
if ($cdGet2.Data) { $g2Url = $cdGet2.Data.url }
Check D5 "CardDAV get/set/sync/clear" ($cdGet0.Status -eq 200 -and $cdSet.Status -eq 204 -and $g1Url -eq $davUrl -and $g1Auth -and $cdSync.Status -eq 200 -and $cdClr.Status -eq 204 -and ($g2Url -eq "" -or $g2Url -eq "x")) "g0=$($cdGet0.Status) set=$($cdSet.Status) g1=$g1Url auth=$g1Auth sync=$($cdSync.Status) clr=$($cdClr.Status) g2='$g2Url'"

Flush-Results "D"
"== done =="
