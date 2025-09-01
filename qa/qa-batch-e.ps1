# qa-batch-e.ps1 — E1..E6 云盘
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== E 云盘 rnd=$rnd =="
# 与 qa-lib 相同的 Base 解析（QA_BASE 优先），供 multipart/下载等裸请求使用
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$s = New-Session
$qa = "qa-drive-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $qa; password = "QaUser2026!"; display_name = "QA Drive" }
$qs = New-Session -Email $qa -Pw "QaUser2026!"

# multipart 上传助手（PS 5.1 手工拼 boundary）
function Invoke-DriveUpload {
    param($Sess, [long]$ParentId, [string]$FileName, [byte[]]$FileBytes)
    $boundary = "----QABoundary" + [guid]::NewGuid().ToString("N")
    $ms = New-Object System.IO.MemoryStream
    $w = New-Object System.IO.BinaryWriter($ms)
    $enc = [Text.Encoding]::UTF8
    $w.Write($enc.GetBytes("--$boundary`r`n"))
    $w.Write($enc.GetBytes("Content-Disposition: form-data; name=`"parent_id`"`r`n`r`n"))
    $w.Write($enc.GetBytes("$ParentId`r`n"))
    $w.Write($enc.GetBytes("--$boundary`r`n"))
    $w.Write($enc.GetBytes("Content-Disposition: form-data; name=`"file`"; filename=`"$FileName`"`r`n"))
    $w.Write($enc.GetBytes("Content-Type: application/octet-stream`r`n`r`n"))
    $w.Write($FileBytes)
    $w.Write($enc.GetBytes("`r`n--$boundary--`r`n"))
    $w.Flush()
    $body = $ms.ToArray()
    $w.Close(); $ms.Close()
    try {
        $r = Invoke-WebRequest -Uri "$Base/drive/upload" -Method POST -Body $body -ContentType "multipart/form-data; boundary=$boundary" -WebSession $Sess.S -UseBasicParsing -TimeoutSec 60
        $json = $null
        try { $json = [Text.Encoding]::UTF8.GetString($r.RawContentStream.ToArray()) | ConvertFrom-Json } catch {}
        return @{ Status = [int]$r.StatusCode; Data = $json; Raw = $r }
    } catch {
        $code = 0
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ Status = $code; Data = $null; Raw = $null; Err = $_.Exception.Message }
    }
}

# 生成确定性测试字节：0..255 两轮 + rnd 标记
$marker = [Text.Encoding]::UTF8.GetBytes("qa-drive-marker-$rnd|")
$content = New-Object byte[] (512 + $marker.Length)
for ($i = 0; $i -lt 512; $i++) { $content[$i] = [byte]($i % 256) }
[Array]::Copy($marker, 0, $content, 512, $marker.Length)

# E1 multipart 上传→树列表可见
$fName = "qa-e1-$rnd.bin"
$up = Invoke-DriveUpload -Sess $qs -ParentId 0 -FileName $fName -FileBytes $content
$fId = $null
if ($up.Data) { $fId = $up.Data.id }
$tree = Api $qs GET "/drive/tree?parent_id=0"
$treeHit = $false
if ($tree.Data) { $treeHit = (@($tree.Data | Where-Object { $_.name -eq $fName -and -not $_.is_dir }).Count -ge 1) }
Check E1 "multipart 上传→树列表可见" ($up.Status -in @(200,201) -and $fId -ne $null -and $tree.Status -eq 200 -and $treeHit) "up=$($up.Status) id=$fId tree=$($tree.Status) hit=$treeHit"

# E2 建目录→重命名→移动文件→移回
$dName = "qa-dir-$rnd"
$mk = Api $qs POST "/drive/folders" @{ parent_id = 0; name = $dName }
$dId = $null
if ($mk.Data) { $dId = $mk.Data.id }
$dNew = "qa-dir-$rnd-r"
$ren = $null
if ($dId) { $ren = Api $qs POST "/drive/rename" @{ id = $dId; name = $dNew } }
$mv = $null
if ($dId -and $fId) { $mv = Api $qs POST "/drive/move" @{ id = $fId; parent_id = $dId } }
$treeIn = Api $qs GET "/drive/tree?parent_id=$dId"
$mvOk = $false
if ($treeIn.Data) { $mvOk = (@($treeIn.Data | Where-Object { $_.name -eq $fName }).Count -ge 1) }
$mvBack = $null
if ($dId -and $fId) { $mvBack = Api $qs POST "/drive/move" @{ id = $fId; parent_id = 0 } }
Check E2 "建目录/重命名/移动文件" ($mk.Status -eq 200 -and $dId -ne $null -and $ren.Status -in @(200,204) -and $mv.Status -in @(200,204) -and $mvOk -and $mvBack.Status -in @(200,204)) "mk=$($mk.Status) ren=$($ren.Status) mv=$($mv.Status) inDir=$mvOk back=$($mvBack.Status)"

# E3 下载内容与上传一致（字节级）
$dl = $null
if ($fId) { $dl = Api $qs GET "/drive/download/$fId" }
$dlOk = $false; $dlLen = -1
if ($dl -and $dl.Raw) {
    $got = $dl.Raw.RawContentStream.ToArray()
    $dlLen = $got.Length
    if ($got.Length -eq $content.Length) {
        $same = $true
        for ($i = 0; $i -lt $content.Length; $i++) { if ($got[$i] -ne $content[$i]) { $same = $false; break } }
        $dlOk = $same
    }
}
Check E3 "下载字节与上传一致" ($dl -ne $null -and $dl.Status -eq 200 -and $dlOk) "dl=$($dl.Status) upLen=$($content.Length) gotLen=$dlLen"

# E4 share 链接：建链→他人带 token 下载→错 token 403→撤链→旧 token 失效
# （/drive/download 挂在 authed 组：匿名一律 401，token 授权面向已登录的其他用户）
$viewer = "qa-drivev-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $viewer; password = "QaUser2026!"; display_name = "QA DriveV" }
$vs = New-Session -Email $viewer -Pw "QaUser2026!"
$sh = $null
if ($fId) { $sh = Api $qs GET "/drive/share/$fId" }
$shUrl = ""; $shTok = ""
if ($sh -and $sh.Data) { $shUrl = $sh.Data.url }
if ($shUrl -match "token=([^&]+)") { $shTok = $Matches[1] }
$vsDl = $null
$vsOk = $false; $vsLen = -1
if ($shTok -ne "") {
    $vsDl = Api $vs GET "/drive/download/${fId}?token=$shTok"
    if ($vsDl -and $vsDl.Raw) {
        $got = $vsDl.Raw.RawContentStream.ToArray()
        $vsLen = $got.Length
        if ($got.Length -eq $content.Length) {
            $same = $true
            for ($i = 0; $i -lt $content.Length; $i++) { if ($got[$i] -ne $content[$i]) { $same = $false; break } }
            $vsOk = $same
        }
    }
}
$vsBad = Api $vs GET "/drive/download/${fId}?token=wrongtoken0000"
$rvk = $null
if ($fId) { $rvk = Api $qs DELETE "/drive/share/$fId" }
$vsAfter = Api $vs GET "/drive/download/${fId}?token=$shTok"
Check E4 "share 链接 建/他人token下载/错token拒/撤后失效" ($sh -ne $null -and $sh.Status -eq 200 -and $shTok -ne "" -and $vsDl.Status -eq 200 -and $vsOk -and $vsBad.Status -eq 403 -and $rvk.Status -eq 204 -and $vsAfter.Status -eq 403) "share=$($sh.Status) dl=$($vsDl.Status)/$vsOk len=$vsLen bad=$($vsBad.Status) rvk=$($rvk.Status) after=$($vsAfter.Status)"

# E5 trash→restore→emptyTrash
$extra = Invoke-DriveUpload -Sess $qs -ParentId 0 -FileName "qa-e5-$rnd.txt" -FileBytes ([Text.Encoding]::UTF8.GetBytes("to trash $rnd"))
$e5Id = $null
if ($extra.Data) { $e5Id = $extra.Data.id }
$tr = $null
if ($e5Id) { $tr = Api $qs POST "/drive/trash" @{ ids = @([int]$e5Id) } }
$trashList = Api $qs GET "/drive/trash"
$inTrash = $false
if ($trashList.Data) { $inTrash = (@($trashList.Data | Where-Object { "$($_.id)" -eq "$e5Id" }).Count -ge 1) }
$rs = $null
if ($e5Id) { $rs = Api $qs POST "/drive/restore" @{ ids = @([int]$e5Id) } }
$tree2 = Api $qs GET "/drive/tree?parent_id=0"
$restored = $false
if ($tree2.Data) { $restored = (@($tree2.Data | Where-Object { "$($_.id)" -eq "$e5Id" }).Count -ge 1) }
$tr2 = $null
if ($e5Id) { $tr2 = Api $qs POST "/drive/trash" @{ ids = @([int]$e5Id) } }
$emp = Api $qs POST "/drive/trash/empty" @{}
$trashList2 = Api $qs GET "/drive/trash"
$trashLeft = 0
if ($trashList2.Data) { $trashLeft = @($trashList2.Data).Count }
Check E5 "trash/restore/emptyTrash" ($tr.Status -in @(200,204) -and $inTrash -and $rs.Status -in @(200,204) -and $restored -and $tr2.Status -in @(200,204) -and $emp.Status -in @(200,204) -and $trashLeft -eq 0) "tr=$($tr.Status) in=$inTrash rs=$($rs.Status) back=$restored emp=$($emp.Status) left=$trashLeft"

# E6 工作台最近文件 = drive 顶层非目录非回收站按 updated_at 倒序前5（与 home 页算法一致）
for ($i = 1; $i -le 6; $i++) {
    $null = Invoke-DriveUpload -Sess $qs -ParentId 0 -FileName "qa-e6-$rnd-$i.txt" -FileBytes ([Text.Encoding]::UTF8.GetBytes("recent $rnd $i"))
}
$tree3 = Api $qs GET "/drive/tree?parent_id=0"
$files = @()
if ($tree3.Data) { $files = @($tree3.Data | Where-Object { -not $_.is_dir -and -not $_.trashed }) }
$sorted = @($files | Sort-Object -Property { [string]$_.updated_at } -Descending:$true)
$top5 = @()
if ($sorted.Count -ge 5) { $top5 = $sorted[0..4] }
$allUploaded = (@($files | Where-Object { "$($_.name)" -like "qa-e6-$rnd-*" }).Count -ge 6)
$names = ($top5 | ForEach-Object { $_.name }) -join ","
Check E6 "顶层文件可按 updated_at 取最近5（工作台同源算法）" ($tree3.Status -eq 200 -and $files.Count -ge 6 -and $top5.Count -eq 5 -and $allUploaded) "tree=$($tree3.Status) files=$($files.Count) top5=$names"

Flush-Results "E"
"== done =="
