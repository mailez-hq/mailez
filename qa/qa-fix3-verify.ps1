# qa-fix3-verify.ps1 - live verification after image rebuild:
#   FIX-A  DEF-TRASH-PURGE   : delete inside Trash actually purges
#   FIX-B  DEF-DRIVE-SHARE-ANON : anonymous share-link download works
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
$env:QA_BASE = "http://127.0.0.1:8081/api/v1"
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
"== fix3 verification rnd=$rnd =="

function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}

$PW = "QaUser2026!"
$A = "qa-f3-a-$rnd@example.com"
$B = "qa-f3-b-$rnd@example.com"
$admin = New-Session
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA f3" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

# ---------------- FIX-A trash purge ----------------
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = "f3-purge-$rnd"; body = "purge me" })
$uid = 0
$deadline = (Get-Date).AddSeconds(60)
while ((Get-Date) -lt $deadline) {
    $rows = @(MsgList (Api $bSess GET "/mail/messages?folder=Inbox&page=0") | Where-Object { $_.subject -match "f3-purge-$rnd" })
    if ($rows.Count -ge 1) { $uid = $rows[0].uid; break }
    Start-Sleep -Seconds 3
}
if ($uid -eq 0) { "FIX-A FAIL deliver (never visible)" } else {
    [void](Api $bSess POST "/mail/delete" @{ folder = "Inbox"; uid = $uid })
    Start-Sleep -Seconds 5
    $trash = @(MsgList (Api $bSess GET "/mail/messages?folder=Trash&page=0") | Where-Object { $_.subject -match "f3-purge-$rnd" })
    if ($trash.Count -ne 1) { "FIX-A FAIL move-to-trash (count=$($trash.Count))" }
    else {
        [void](Api $bSess POST "/mail/delete" @{ folder = "Trash"; uid = $trash[0].uid })
        $gone = $false
        $deadline = (Get-Date).AddSeconds(90)
        while ((Get-Date) -lt $deadline) {
            $trash = @(MsgList (Api $bSess GET "/mail/messages?folder=Trash&page=0") | Where-Object { $_.subject -match "f3-purge-$rnd" })
            if ($trash.Count -eq 0) { $gone = $true; break }
            Start-Sleep -Seconds 10
        }
        if ($gone) { "FIX-A PASS trash purge" } else { "FIX-A FAIL still in Trash after 90s" }
    }
}

# ---------------- FIX-B drive share anon ----------------
$bound = "f3 share probe $rnd"
$bnd = "qb"
$body = "--$bnd`r`nContent-Disposition: form-data; name=`"file`"; filename=`"f3-$rnd.txt`"`r`nContent-Type: text/plain`r`n`r`n$bound`r`n--$bnd--`r`n"
$upStatus = 0; $fileId = 0
try {
    $up = Invoke-WebRequest -Uri "$env:QA_BASE/drive/upload" -Method POST -Body $body -ContentType "multipart/form-data; boundary=$bnd" -WebSession $bSess.S -UseBasicParsing -TimeoutSec 60
    $upStatus = [int]$up.StatusCode
    $upJson = [Text.Encoding]::UTF8.GetString($up.RawContentStream.ToArray()) | ConvertFrom-Json
    $fileId = $upJson.id
} catch { if ($_.Exception.Response) { $upStatus = [int]$_.Exception.Response.StatusCode } }
if (-not $fileId) { "FIX-B FAIL upload -> $upStatus" }
else {
    $share = Api $bSess GET "/drive/share/$fileId"
    $url = ""
    if ($share.Data) {
        if ($share.Data.url) { $url = "$($share.Data.url)" } elseif ($share.Data.token) { $url = "$($share.Data.token)" }
    }
    if (-not $url) { "FIX-B FAIL share -> $($share.Status) $($share.Body)" }
    else {
        $full = $url
        if ($full -notmatch "^http") { $full = "http://127.0.0.1:8081$full" }
        $anonStatus = 0; $anonOk = $false
        try {
            $an = Invoke-WebRequest -Uri $full -UseBasicParsing -TimeoutSec 30
            $anonStatus = [int]$an.StatusCode
            $anonOk = ([Text.Encoding]::UTF8.GetString($an.RawContentStream.ToArray()) -eq $bound)
        } catch { if ($_.Exception.Response) { $anonStatus = [int]$_.Exception.Response.StatusCode } }
        if ($anonStatus -eq 200 -and $anonOk) { "FIX-B PASS anonymous share download" }
        else { "FIX-B FAIL anon status=$anonStatus contentMatch=$anonOk" }
        # revoke -> link must die
        $rv = Api $bSess DELETE "/drive/share/$fileId"
        $after = 0
        try { $an2 = Invoke-WebRequest -Uri $full -UseBasicParsing -TimeoutSec 30; $after = [int]$an2.StatusCode } catch { if ($_.Exception.Response) { $after = [int]$_.Exception.Response.StatusCode } else { $after = 0 } }
        if ($rv.Status -in @(200,204) -and $after -in @(401,403,404,410)) { "FIX-B PASS revoked link is dead ($after)" } else { "FIX-B WARN revoke=$($rv.Status) after=$after" }
    }
    [void](Api $bSess POST "/drive/trash" @{ ids = @($fileId) })
    [void](Api $bSess POST "/drive/trash/empty" @{ ids = @() })
}

# cleanup
foreach ($u in @($A, $B)) {
    $d = Api $admin DELETE "/users/$u"
    if ($d.Status -notin @(200, 204)) { "cleanup: delete $u -> $($d.Status) $($d.Body)" }
}
"cleanup done"
