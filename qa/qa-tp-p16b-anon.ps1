# qa-tp-p16b-anon.ps1 - focused probe: anonymous alias creation returns 403
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P16.3 anon alias probe rnd=$rnd =="
function Body($r) { if ($r.Raw) { return ([Text.Encoding]::UTF8.GetString($r.Raw.RawContentStream.ToArray())) } ; return "" }

$admin = New-Session
$PW = "QaUser2026!"
$D = "qa-p16c-$rnd.example.com"
$U = "qa-p16c-u-$rnd@example.com"
[void](Api $admin POST "/domains" @{ name = $D })
$mk = Api $admin POST "/users" @{ email = $U; password = $PW; display_name = "QA P16c" }
"create user=$($mk.Status)"
$put = Api $admin PUT "/domains/$D" @{ anonmail_enabled = $true }
"enable anonmail status=$($put.Status) body=$(Body $put)"
$row = docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT CONCAT('anonmail=',anonmail_enabled) FROM domain WHERE name='$D';" 2>$null | Select-Object -First 1
"db row: $row"

$sess = New-Session $U $PW
$r1 = Api $sess POST "/anon-aliases" @{ domain = $D; display_name = "qa probe" }
"user create anon status=$($r1.Status) body=$(Body $r1)"
$r2 = Api $admin POST "/anon-aliases" @{ domain = $D; display_name = "qa probe admin" }
"admin create anon status=$($r2.Status) body=$(Body $r2)"
$domains = Api $sess GET "/anon-aliases/domains"
"anon domains for user=$($domains.Status) body=$(Body $domains)"
$list = Api $sess GET "/anon-aliases"
"user anon list=$($list.Status) body=$(Body $list)"

$ud = Api $admin DELETE "/users/$U"
$dd = Api $admin DELETE "/domains/$D"
"cleanup: user=$($ud.Status) domain=$($dd.Status)"
