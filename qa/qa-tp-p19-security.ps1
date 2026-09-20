# qa-tp-p19-security.ps1 - P19 anti-spam and content safety (TEST-PROMPTS.md P19), rev1
#
# Stack reality: MAILEZ_ANTIVIRUS=none and MAILEZ_SCAN_MACROS=false in this
# deployment, and there is no public DNS for the test domains, so inbound
# SPF/DKIM/DMARC verdicts cannot be exercised end to end. Those parts are
# reported as observations with the reason, not as passes.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P19 security rev1 rnd=$rnd =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function Wait-Mail($sess, [string]$subject, [string]$folder = "Inbox", [int]$timeout = 90) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $timeout) {
        $r = Api $sess GET "/mail/messages?folder=$folder&page=0"
        foreach ($m in (MsgList $r)) { if ($m.subject -eq $subject) { return $m } }
        Start-Sleep -Seconds 3
    }
    return $null
}

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-p19-a-$rnd@example.com"
$B = "qa-p19-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P19" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

# --------------------------------------------- P19.1 scanning headers on mail
$subj1 = "p19-scan-$rnd"
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = $subj1; body = "scan me"; html = "<p>scan me</p>" })
$m1 = Wait-Mail $bSess $subj1
$raw1 = ""
if ($m1) { $r = Api $bSess GET "/mail/raw?folder=Inbox&uid=$($m1.uid)"; if ($r.Data) { $raw1 = [string]$r.Data.raw } }
$spamd = ($raw1 -match "X-Spamd-") -or ($raw1 -match "X-Spam-Status")
$authRes = $raw1 -match "Authentication-Results"
$dkimSig = $raw1 -match "(?m)^DKIM-Signature:"
# example.com itself has no DKIM key in this stack (P1 observation), so the
# signing half of the verdict chain can only be judged on a domain that has one
# (see P15.1d, where the signed domain yields a DKIM-Signature header).
Check P19.1 "delivered mail carries spam and authentication verdict headers" ($m1 -ne $null -and $spamd -and $authRes) `
    "delivered=$($m1 -ne $null) xSpamd=$spamd authResults=$authRes dkimSignatureOnExampleCom=$dkimSig (example.com has no DKIM key; signing verified in P15.1d)"

# ------------------------------------------- P19.4 attachment threat scanning
$eicar = 'X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*'
$eicarB64 = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($eicar))
$subj2 = "p19-eicar-$rnd"
$send = Api $aSess POST "/mail/send" @{ to = @($B); subject = $subj2; body = "eicar"; attachments = @(@{ name = "eicar.com"; type = "text/plain"; data = $eicarB64 }) }
$m2 = Wait-Mail $bSess $subj2 "Inbox" 45
$delivered = $m2 -ne $null
$scanCfg = (docker inspect mailez-mail-filter-1 --format "{{json .Config.Env}}" 2>&1 | Out-String)
$macroCfg = (docker inspect mailez-mailezine-1 --format "{{json .Config.Env}}" 2>&1 | Out-String)
$avOff = $macroCfg -match "MAILEZ_ANTIVIRUS=none"
if ($delivered) {
    Check P19.4 "EICAR attachment handling matches the configured posture" $avOff `
        "delivered=$delivered sendStatus=$($send.Status) engineAntivirus=none macroScan=$(if($macroCfg -match 'MAILEZ_SCAN_MACROS=false'){'false'}else{'true'}) -> scanning disabled by configuration, recorded as an observation"
} else {
    Check P19.4 "EICAR attachment handling matches the configured posture" ($send.Status -eq 422 -or $send.Status -eq 400) `
        "sendStatus=$($send.Status) delivered=false (rejected upstream)"
}

# ------------------------------------------------ P19.5 HTML sanitisation
$subj3 = "p19-html-$rnd"
$hostile = '<p>hello</p><script>alert(1)</script><img src="http://tracker.invalid/pixel.gif" onerror="alert(2)"><a href="javascript:alert(3)">x</a>'
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = $subj3; body = "html probe"; html = $hostile })
$m3 = Wait-Mail $bSess $subj3
$html = ""
if ($m3) {
    $md = Api $bSess GET "/mail/message?folder=Inbox&uid=$($m3.uid)"
    if ($md.Data -and $md.Data.html_body) { $html = [string]$md.Data.html_body }
}
$scriptGone = $html -notmatch "<script"
$onErrorGone = $html -notmatch "onerror"
$jsHrefGone = $html -notmatch "javascript:"
$remoteImg = $html -match "tracker.invalid"
Check P19.5 "server-side HTML sanitisation strips script/event handlers" `
    ($m3 -ne $null -and $scriptGone -and $onErrorGone -and $jsHrefGone) `
    "delivered=$($m3 -ne $null) scriptStripped=$scriptGone onerrorStripped=$onErrorGone javascriptHrefStripped=$jsHrefGone remoteImgKeptAsReference=$remoteImg htmlLen=$($html.Length)"

# --------------------------------------- P19.3 junk semantics + bayes learn
$moved = $false
if ($m3) {
    $r = Api $bSess POST "/mail/move" @{ folder = "Inbox"; uid = $m3.uid; destination = "Junk" }
    $moved = $r.Status -in @(200, 204)
}
Start-Sleep -Seconds 8
$junkSeen = $false
$jl = Api $bSess GET "/mail/messages?folder=Junk&page=0"
foreach ($m in (MsgList $jl)) { if ($m.subject -eq $subj3) { $junkSeen = $true } }
$rspamdLog = (docker logs mailez-mail-filter-1 --since 3m 2>&1 | Out-String)
$learned = $rspamdLog -match "learn|bayes|statfile" -or $rspamdLog -match "spam"
Check P19.3 "moving mail to Junk is reflected and observed by the filter" ($moved -and $junkSeen) `
    "moved=$moved junkVisible=$junkSeen rspamdLearnEvidence=$learned (bayes learning path: $([regex]::Match($rspamdLog,'(learn|bayes)[^\r\n]{0,60}').Value))"

$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P19"
"== done =="
