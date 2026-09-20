# qa-quota-probe.ps1 - separate the two oversized-mail paths:
#   (a) a big body made of short lines  -> recipient quota refusal (552)
#   (b) a big body on ONE long line     -> SMTP line-length refusal (554)
# Both used to surface as a bare 502; the fix should give each an actionable
# 4xx instead.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== quota / line-length probe rnd=$rnd =="
function Body($r) {
    if ($r.Body) { return [string]$r.Body }
    if ($r.Raw) { return [Text.Encoding]::UTF8.GetString($r.Raw.RawContentStream.ToArray()) }
    return ""
}
function Json($r) { if ($r.Data) { return $r.Data } ; try { return ((Body $r) | ConvertFrom-Json) } catch { return $null } }
function MsgList($r) {
    if ($null -eq $r.Data) { return @() }
    if (@($r.Data.PSObject.Properties.Name) -contains "data") { return @($r.Data.data) }
    return @($r.Data)
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-qp-a-$rnd@example.com"
$Q = "qa-qp-q-$rnd@example.com"
foreach ($u in @($A, $Q)) { [void](Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA quota probe" }) }
[void](Api $admin PUT "/users/$Q" @{ quota_bytes = 200000 })
$aSess = New-Session $A $PW

# (a) oversized but well-folded body: only the quota can reject this
$line = "0123456789" * 7                  # 70 chars per line
$manyLines = (($line + "`r`n") * 4300)    # ~300 KB, every line well under 1000
# NOTE: response variables must not be named $a/$b — PowerShell is
# case-insensitive and would overwrite the $A/$B addresses.
$quotaResp = Api $aSess POST "/mail/send" @{ to = @($Q); subject = "qp-quota-$rnd"; body = $manyLines }
$quotaJson = Json $quotaResp
"quota path: status=$($quotaResp.Status) code=$($quotaJson.code) error=$($quotaJson.error)"
Check QP.1 "well-folded oversized mail is refused as a quota problem" ($quotaResp.Status -eq 422 -and "$($quotaJson.code)" -eq "recipient_quota_exceeded") `
    "status=$($quotaResp.Status) code=$($quotaJson.code) error=$($quotaJson.error)"

# (b) one very long line: the SMTP line limit, not the quota
$lineResp = Api $aSess POST "/mail/send" @{ to = @($A); subject = "qp-line-$rnd"; body = ("x" * 300000) }
$lineJson = Json $lineResp
"long-line path: status=$($lineResp.Status) code=$($lineJson.code) error=$($lineJson.error)"
# The fix belongs on the sending side: fold the line instead of letting the
# engine reject it. So the assertion is "it goes out, and no wire line is
# over-long, and the payload survives".
$delivered = $null
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 90 -and -not $delivered) {
    foreach ($m in (MsgList (Api $aSess GET "/mail/messages?folder=Inbox&page=0"))) {
        if ($m.subject -eq "qp-line-$rnd") { $delivered = $m }
    }
    if (-not $delivered) { Start-Sleep -Seconds 3 }
}
$rawText = ""
$maxLine = 0
$payloadSeen = $false
if ($delivered) {
    $rawResp = Api $aSess GET "/mail/raw?folder=Inbox&uid=$($delivered.uid)"
    if ($rawResp.Data) { $rawText = [string]$rawResp.Data.raw }
    foreach ($l in ($rawText -split "`r?`n")) { if ($l.Length -gt $maxLine) { $maxLine = $l.Length } }
    $payloadSeen = $rawText -match "xxxxx"
}
Check QP.2 "an unbroken 300k-char body is folded and delivered (was 554/502)" `
    ($lineResp.Status -in @(200, 204) -and $delivered -ne $null -and $maxLine -le 1000 -and $payloadSeen) `
    "send=$($lineResp.Status) delivered=$($delivered -ne $null) longestRawLine=$maxLine (limit 998+CRLF) payloadPresent=$payloadSeen rawBytes=$($rawText.Length)"

$log = docker logs mailez-backend-1 --since 3m 2>&1 | Out-String
$quotaErr = ([regex]::Match($log, "api error[^\r\n]*(mailbox full|quota)[^\r\n]*")).Value
$lineErr = ([regex]::Match($log, "api error[^\r\n]*line too long[^\r\n]*")).Value
"backend log: quota=[$($quotaErr.Trim())] line=[$($lineErr.Trim())]"

$cl = @()
foreach ($u in @($A, $Q)) { $cl += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cl -join ' ')"
Flush-Results "QP"
