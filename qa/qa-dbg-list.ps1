# scratch probe: what does /mail/messages actually return for one account?
param(
    [Parameter(Mandatory = $true)][string]$Email,
    [string]$Pw = "QaUser2026!",
    [string]$Folder = "INBOX"
)
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$env:QA_BASE = if ($env:QA_BASE) { $env:QA_BASE } else { "http://127.0.0.1:8081/api/v1" }
$s = New-Session $Email $Pw
"login=$($s.Status)"
function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
for ($i = 1; $i -le 3; $i++) {
    $r = Api $s GET "/mail/messages?folder=$Folder&page=0"
    $total = if ($r.Raw) { $r.Raw.Headers["X-Total-Messages"] } else { "" }
    $subs = @()
    foreach ($m in (MsgList $r)) { $subs += "$($m.uid):$($m.subject)" }
    "try$i status=$($r.Status) x-total=$total n=$((MsgList $r).Count) [$($subs -join ' | ')]"
    Start-Sleep -Seconds 2
}
