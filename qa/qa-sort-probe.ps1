# scratch probe: which sort keys does the API actually serve, and how fast?
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$env:QA_BASE = if ($env:QA_BASE) { $env:QA_BASE } else { "http://127.0.0.1:8081/api/v1" }
function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}
$s = New-Session
foreach ($p in @(
        "/mail/messages?folder=Inbox&page=0",
        "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc",
        "/mail/messages?folder=Inbox&page=0&sort=subject&dir=desc",
        "/mail/messages?folder=Inbox&page=0&sort=subject&dir=asc",
        "/mail/messages?folder=Inbox&page=0&sort=from&dir=desc",
        "/mail/messages?folder=Inbox&page=0&sort=size&dir=desc")) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $r = Api $s GET $p
    $ms = [int]$sw.Elapsed.TotalMilliseconds
    $t = if ($r.Raw) { $r.Raw.Headers["X-Total-Messages"] } else { "" }
    $tail = $p -replace "/mail/messages\?folder=Inbox&page=0", ""
    "sort=$tail status=$($r.Status) rows=$((MsgList $r).Count) x-total=$t ${ms}ms"
}
