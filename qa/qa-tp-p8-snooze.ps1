# qa-tp-p8-snooze.ps1 - P8 snooze (TEST-PROMPTS.md P8), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P8 snooze rev1 rnd=$rnd base=$Base =="
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
    param([string]$Who, [string]$Method, [string]$Path, $Body = $null)
    $s = Get-S $Who
    if ($null -eq $s) { return @{ Status = 0; Data = $null; Body = "no-session" } }
    $r = Api @{ S = $s } $Method $Path $Body
    if ($r.Status -eq 401) {
        $script:Auth401++
        $Sessions[$Who].S = $null
        $s2 = Get-S $Who
        if ($null -eq $s2) { return @{ Status = 401; Data = $null; Body = "relogin-failed" } }
        $r = Api @{ S = $s2 } $Method $Path $Body
    }
    return $r
}
$U = "qa-p8a-$rnd@example.com"
$admin = New-Session
$r = Api $admin POST "/users" @{ email = $U; password = "QaUser2026!"; display_name = "QA P8" }
if ($r.Status -notin @(200,201)) { "prep FAIL create -> $($r.Status)"; exit 1 }
Bind-Session "A" $U "QaUser2026!"
"prep: user ready"
function Find-Uid {
    param([string]$Subject)
    $r = Invoke-Api "A" GET "/mail/messages?folder=Inbox&page=0"
    if ($r.Status -ne 200) { return 0 }
    foreach ($m in @($r.Data)) { if ($m.subject -eq $Subject) { return [uint32]$m.uid } }
    return 0
}
function Wait-Uid {
    param([string]$Subject, [int]$Seconds)
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
        $u = Find-Uid $Subject
        if ($u -gt 0) { return @{ Uid = $u; Ms = [int]$sw.ElapsedMilliseconds } }
        Start-Sleep -Seconds 5
    }
    return @{ Uid = 0; Ms = [int]$sw.ElapsedMilliseconds }
}
function Send-Self {
    param([string]$Subject, [string]$Text)
    return (Invoke-Api "A" POST "/mail/send" @{ to = @($U); subject = $Subject; body = $Text }).Status
}
# seed
$m1 = "P8-snooze-$rnd"; $m2 = "P8-cancel-$rnd"; $m3 = "P8-double-$rnd"
[void](Send-Self $m1 "body one")
[void](Send-Self $m2 "body two")
[void](Send-Self $m3 "body three")
$u1 = (Wait-Uid $m1 150).Uid
$u2 = (Wait-Uid $m2 150).Uid
$u3 = (Wait-Uid $m3 150).Uid
"seed: u1=$u1 u2=$u2 u3=$u3"
if ($u1 -eq 0 -or $u2 -eq 0 -or $u3 -eq 0) { "seed incomplete; abort"; Flush-Results "P8"; exit 1 }

# P8.1 snooze +60s: leaves Inbox, appears in snoozed with metadata
$until = [int][DateTimeOffset]::UtcNow.AddSeconds(60).ToUnixTimeSeconds()
$sz = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Inbox"; uid = $u1; until = $until }
$leftInbox = $false; $inSnoozed = $false; $d81 = "snooze=$($sz.Status)"
foreach ($i in 1..12) {
    Start-Sleep -Seconds 5
    if ((Find-Uid $m1) -eq 0) { $leftInbox = $true; $d81 += " leftInboxAfter=~$($i*5)s" }
    $sl = Invoke-Api "A" GET "/mail/snoozed"
    if ($sl.Data) {
        foreach ($e in @($sl.Data)) {
            $es = "$($e.subject)"
            if ($es -eq $m1) { $inSnoozed = $true; $d81 += " snoozedEntry={subject:$es,folder:$($e.folder),until:$($e.until)$($e.wake_at)}" }
        }
    }
    if ($leftInbox -and $inSnoozed) { break }
}
Check P8.1 "snooze leaves Inbox, appears in snoozed" ($sz.Status -eq 204 -and $leftInbox -and $inSnoozed) "$d81 (uid=$u1)"

# P8.2 wake on time: returns to Inbox, leaves snoozed
$w = Wait-Uid $m1 240
$slAfter = Invoke-Api "A" GET "/mail/snoozed"
$stillSnoozed = $false
if ($slAfter.Data) { foreach ($e in @($slAfter.Data)) { if ("$($e.subject)" -eq $m1) { $stillSnoozed = $true } } }
Check P8.2 "wake returns message to Inbox on time" ($w.Uid -gt 0 -and -not $stillSnoozed) "reappearedAfter=$($w.Ms)ms since-poll-start stillSnoozed=$stillSnoozed"

# P8.3 cancel snooze (post without until) returns immediately
$fut = [int][DateTimeOffset]::UtcNow.AddSeconds(300).ToUnixTimeSeconds()
$sz2 = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Inbox"; uid = $u2; until = $fut }
$gone2 = $false
foreach ($i in 1..10) { Start-Sleep -Seconds 4; if ((Find-Uid $m2) -eq 0) { $gone2 = $true; break } }
$wk = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Inbox"; uid = $u2 }
$back = Wait-Uid $m2 90
$sl3 = Invoke-Api "A" GET "/mail/snoozed"
$cleared3 = $true
if ($sl3.Data) { foreach ($e in @($sl3.Data)) { if ("$($e.subject)" -eq $m2) { $cleared3 = $false } } }
Check P8.3 "manual wake returns message immediately" ($sz2.Status -eq 204 -and $gone2 -and $wk.Status -eq 204 -and $back.Uid -gt 0 -and $cleared3) "snooze=$($sz2.Status) gone=$gone2 wake=$($wk.Status) backIn=$($back.Ms)ms cleared=$cleared3"

# P8.4 double snooze on already-snoozed message
$fut4a = [int][DateTimeOffset]::UtcNow.AddSeconds(300).ToUnixTimeSeconds()
$sz4a = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Inbox"; uid = $u3; until = $fut4a }
$fut4b = [int][DateTimeOffset]::UtcNow.AddSeconds(400).ToUnixTimeSeconds()
$sz4b = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Inbox"; uid = $u3; until = $fut4b }
$sl4 = Invoke-Api "A" GET "/mail/snoozed"
$cnt4 = 0
if ($sl4.Data) { foreach ($e in @($sl4.Data)) { if ("$($e.subject)" -eq $m3) { $cnt4++ } } }
# clean wake
$wk4 = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Inbox"; uid = $u3 }
Check P8.4 "double snooze accepted, single entry" ($sz4a.Status -eq 204 -and $sz4b.Status -in @(200,204) -and $cnt4 -eq 1 -and $wk4.Status -eq 204) "first=$($sz4a.Status) second=$($sz4b.Status) entries=$cnt4 wake=$($wk4.Status)"

# P8.5 snooze a message in Trash: record behavior (allow or clean reject, no 5xx)
$dl = Invoke-Api "A" POST "/mail/delete" @{ folder = "Inbox"; uid = $u1 }
$tu1 = 0
foreach ($i in 1..10) { Start-Sleep -Seconds 4; $rt = Invoke-Api "A" GET "/mail/messages?folder=Trash&page=0"; if ($rt.Status -eq 200) { foreach ($m in @($rt.Data)) { if ($m.subject -eq $m1) { $tu1 = [uint32]$m.uid; break } } }; if ($tu1 -gt 0) { break } }
$szT = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Trash"; uid = $tu1; until = $fut }
$no5xx = ($szT.Status -lt 500)
$cleanT = Invoke-Api "A" POST "/mail/snooze" @{ folder = "Trash"; uid = $tu1 }
Check P8.5 "snooze on Trash message: allow or clean reject, no 5xx" ($dl.Status -eq 204 -and $no5xx) "trashDel=$($dl.Status) trashUid=$tu1 snoozeStatus=$($szT.Status) body=$("$($szT.Body)".Trim())"

"== auth401 self-heals: $script:Auth401 =="
$ud = Api $admin DELETE "/users/$U"
"cleanup: user=$($ud.Status)"
Flush-Results "P8"
"== done =="
