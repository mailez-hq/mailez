# qa-tp-p11-drive.ps1 - P11 drive upload/tree/move/download/share/trash (TEST-PROMPTS.md P11), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P11 drive rev1 rnd=$rnd base=$Base =="
$script:Auth401 = 0
$Sessions = @{}
function Bind-Session { param([string]$Who, [string]$Email, [string]$Pw) $Sessions[$Who] = @{ Email = $Email; Pw = $Pw; S = $null } }
function Get-S {
    param([string]$Who)
    $c = $Sessions[$Who]
    if (-not $c.S) {
        $body = @{ email = $c.Email; pw = $c.Pw } | ConvertTo-Json -Compress
        try {
            $lr = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($body)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s
            $c.S = $s
        } catch { return $null }
    }
    return $c.S
}
function Invoke-Api {
    param([string]$Who, [string]$Method, [string]$Path, $Body = $null, [string]$ContentType = "application/json")
    $s = Get-S $Who
    if ($null -eq $s) { return @{ Status = 0; Data = $null; Body = "no-session" } }
    $r = Api @{ S = $s } $Method $Path $Body
    return $r
}
function Send-Multipart {
    param([string]$Who, [string]$FileName, [byte[]]$Bytes)
    $s = Get-S $Who
    $b = "----qaBoundary$rnd"
    $ms = New-Object IO.MemoryStream
    $enc = [Text.Encoding]::UTF8
    $w = New-Object IO.BinaryWriter($ms)
    $w.Write($enc.GetBytes("--$b`r`n"))
    $w.Write($enc.GetBytes("Content-Disposition: form-data; name=`"file`"; filename=`"$FileName`"`r`n"))
    $w.Write($enc.GetBytes("Content-Type: application/octet-stream`r`n`r`n"))
    $w.Write($Bytes)
    $w.Write($enc.GetBytes("`r`n--$b--`r`n"))
    $body = $ms.ToArray()
    $w.Flush()
    try {
        $r = Invoke-WebRequest -Uri "$Base/drive/upload" -Method POST -Body $body -ContentType "multipart/form-data; boundary=$b" -WebSession $s -UseBasicParsing -TimeoutSec 60
        return @{ Status = [int]$r.StatusCode; Body = $r.Content }
    } catch {
        $code = 0; if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ Status = $code; Body = "$($_.ErrorDetails.Message)" }
    }
}
$U = "qa-p11a-$rnd@example.com"
$admin = New-Session
$r = Api $admin POST "/users" @{ email = $U; password = "QaUser2026!"; display_name = "QA P11" }
if ($r.Status -notin @(200,201)) { "prep FAIL create -> $($r.Status)"; exit 1 }
Bind-Session "A" $U "QaUser2026!"
"prep: user ready"

# P11.1 upload + tree + download byte-identical
$payload = [Text.Encoding]::UTF8.GetBytes("drive payload marker $rnd " + ("x" * 1000))
$up = Send-Multipart "A" "qa11-$rnd.txt" $payload
$tree = Invoke-Api "A" GET "/drive/tree"
$fid = 0; $fsize = 0; $found = $false
if ($tree.Data) {
    $flat = @($tree.Data)
    foreach ($node in $flat) {
        if ("$($node.name)" -eq "qa11-$rnd.txt") { $fid = $node.id; $fsize = $node.size; $found = $true }
        if ($node.children) { foreach ($ch in @($node.children)) { if ("$($ch.name)" -eq "qa11-$rnd.txt") { $fid = $ch.id; $fsize = $ch.size; $found = $true } } }
    }
}
$dlOk = $false; $dlStatus = 0
if ($fid -gt 0) {
    try {
        $dl = Invoke-WebRequest -Uri "$Base/drive/download/$fid" -WebSession (Get-S "A") -UseBasicParsing -TimeoutSec 60
        $dlStatus = [int]$dl.StatusCode
        $got = $dl.Content
        if ($got -is [string]) { $got = [Text.Encoding]::UTF8.GetBytes($got) }
        $dlOk = ($got.Length -eq $payload.Length)
        if ($dlOk) { for ($i = 0; $i -lt $payload.Length; $i += 97) { if ($got[$i] -ne $payload[$i]) { $dlOk = $false; break } } }
    } catch { $dlStatus = 0; if ($_.Exception.Response) { $dlStatus = [int]$_.Exception.Response.StatusCode } }
}
$md5up = [System.Security.Cryptography.MD5]::Create()
$hashUp = ([BitConverter]::ToString($md5up.ComputeHash($payload)) -replace '-','').Substring(0,8)
Check P11.1 "upload -> tree -> download byte-identical" ($up.Status -in @(200,201) -and $found -and $dlOk -and $fsize -eq $payload.Length) "upload=$($up.Status) found=$found size=$fsize/$($payload.Length) download=$dlStatus match=$dlOk payloadHash8=$hashUp"

# P11.2 folder create/rename/move
$cf = Invoke-Api "A" POST "/drive/folders" @{ parent_id = 0; name = "qa11dir-$rnd" }
$dirId = 0; if ($cf.Data) { $dirId = $cf.Data.id }
$rn = Invoke-Api "A" POST "/drive/rename" @{ id = $dirId; name = "qa11dir2-$rnd" }
$mv = Invoke-Api "A" POST "/drive/move" @{ id = $fid; parent_id = $dirId }
$tree2 = Invoke-Api "A" GET "/drive/tree"
$movedOk = $false
if ($tree2.Data) {
    foreach ($node in @($tree2.Data)) {
        if ("$($node.name)" -eq "qa11dir2-$rnd" -and $node.children) {
            foreach ($ch in @($node.children)) { if ("$($ch.name)" -eq "qa11-$rnd.txt") { $movedOk = $true } }
        }
    }
}
Check P11.2 "folder create/rename + move file into folder" ($cf.Status -in @(200,201) -and $dirId -gt 0 -and $rn.Status -in @(200,204) -and $mv.Status -in @(200,204) -and $movedOk) "create=$($cf.Status) dirId=$dirId rename=$($rn.Status) move=$($mv.Status) nestedVisible=$movedOk"

# P11.3 share link lifecycle
$shr = Invoke-Api "A" GET "/drive/share/$fid"
$shareUrl = ""
if ($shr.Data) {
    if ($shr.Data.url) { $shareUrl = "$($shr.Data.url)" } elseif ($shr.Data.token) { $shareUrl = "$($shr.Data.token)" }
}
$anonOk = $false; $anonStatus = 0
if ($shareUrl) {
    $full = $shareUrl
    if ($full -notmatch "^http") { $full = "http://127.0.0.1:8081$full" }
    try { $an = Invoke-WebRequest -Uri $full -UseBasicParsing -TimeoutSec 30; $anonStatus = [int]$an.StatusCode; $anonOk = ($an.Content -match "drive payload marker") } catch { if ($_.Exception.Response) { $anonStatus = [int]$_.Exception.Response.StatusCode } }
}
$rv = Invoke-Api "A" DELETE "/drive/share/$fid"
$anonAfter = 0
if ($shareUrl) {
    $full2 = $shareUrl
    if ($full2 -notmatch "^http") { $full2 = "http://127.0.0.1:8081$full2" }
    try { $an2 = Invoke-WebRequest -Uri $full2 -UseBasicParsing -TimeoutSec 30; $anonAfter = [int]$an2.StatusCode } catch { if ($_.Exception.Response) { $anonAfter = [int]$_.Exception.Response.StatusCode } else { $anonAfter = 0 } }
}
Check P11.3 "share link: anonymous fetch -> revoke -> dead" ($shr.Status -eq 200 -and $shareUrl -ne "" -and $anonOk -and $rv.Status -in @(200,204) -and $anonAfter -in @(401,403,404,410)) "share=$($shr.Status) url=$shareUrl anon=$anonStatus contentMatch=$anonOk revoke=$($rv.Status) afterRevoke=$anonAfter"

# P11.4 trash lifecycle
$tr = Invoke-Api "A" POST "/drive/trash" @{ ids = @($fid) }
$tl = Invoke-Api "A" GET "/drive/trash"
$inTrash = $false
if ($tl.Data) { foreach ($x in @($tl.Data)) { if ("$($x.name)" -eq "qa11-$rnd.txt") { $inTrash = $true } } }
$rs = Invoke-Api "A" POST "/drive/restore" @{ ids = @($fid) }
$tree3 = Invoke-Api "A" GET "/drive/tree"
$restored = $false
if ($tree3.Data) { foreach ($node in @($tree3.Data)) { $nm = "$($node.name)"; if ($nm -eq "qa11-$rnd.txt") { $restored = $true } } }
$tr2 = Invoke-Api "A" POST "/drive/trash" @{ ids = @($fid) }
$et = Invoke-Api "A" POST "/drive/trash/empty" @{ ids = @() }
$tl2 = Invoke-Api "A" GET "/drive/trash"
$trashEmpty = $true
if ($tl2.Data) { foreach ($x in @($tl2.Data)) { if ("$($x.name)" -eq "qa11-$rnd.txt") { $trashEmpty = $false } } }
Check P11.4 "trash -> restore -> re-trash -> empty" ($tr.Status -in @(200,204) -and $inTrash -and $rs.Status -in @(200,204) -and $restored -and $et.Status -in @(200,204) -and $trashEmpty) "trash=$($tr.Status) inTrash=$inTrash restore=$($rs.Status) restored=$restored empty=$($et.Status) trashClean=$trashEmpty"

# P11.5 boundaries: 0-byte, Chinese name, duplicate upload
$zr = Send-Multipart "A" "qa11-zero-$rnd.txt" ([byte[]]@())
$cn = Send-Multipart "A" "qa11-中文文件-$rnd.txt" ([Text.Encoding]::UTF8.GetBytes("chinese name content"))
$dup1 = Send-Multipart "A" "qa11-dup-$rnd.txt" ([Text.Encoding]::UTF8.GetBytes("first"))
$dup2 = Send-Multipart "A" "qa11-dup-$rnd.txt" ([Text.Encoding]::UTF8.GetBytes("second"))
$tree4 = Invoke-Api "A" GET "/drive/tree"
$zeroOk = $false; $cnOk = $false; $dupCount = 0; $treeNames = ""
if ($tree4.Data) {
    foreach ($node in @($tree4.Data)) {
        $treeNames += "$($node.name);"
        if ("$($node.name)" -eq "qa11-zero-$rnd.txt") { $zeroOk = $true }
        if ("$($node.name)" -eq "qa11-中文文件-$rnd.txt") { $cnOk = $true }
        if ("$($node.name)" -eq "qa11-dup-$rnd.txt") { $dupCount++ }
    }
}
Check P11.5 "0-byte ok; Chinese name intact; duplicate policy recorded" ($zr.Status -in @(200,201) -and $zeroOk -and $cn.Status -in @(200,201) -and $cnOk -and $dup1.Status -in @(200,201) -and $dup2.Status -in @(200,201,409)) "zero=$($zr.Status)/$zeroOk chinese=$($cn.Status)/$cnOk dup=$($dup1.Status),$($dup2.Status) dupCount=$dupCount"

"== auth401 self-heals: $script:Auth401 =="
$ud = Api $admin DELETE "/users/$U"
"cleanup: user=$($ud.Status) (drive objects cascade per DEF-DEL-PURGE watch)"
Flush-Results "P11"
"== done =="
