# qa-tp-p10-calendar.ps1 - P10 calendar/ICS/shares (TEST-PROMPTS.md P10), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P10 calendar rev1 rnd=$rnd base=$Base =="
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
$U = "qa-p10a-$rnd@example.com"
$P = "qa-p10b-$rnd@example.com"
$admin = New-Session
foreach ($x in @($U,$P)) {
    $r = Api $admin POST "/users" @{ email = $x; password = "QaUser2026!"; display_name = "QA P10" }
    if ($r.Status -notin @(200,201)) { "prep FAIL create -> $($r.Status)"; exit 1 }
}
Bind-Session "A" $U "QaUser2026!"
Bind-Session "B" $P "QaUser2026!"
"prep: users ready"

$start = [DateTimeOffset]::UtcNow.AddHours(1).ToString("yyyy-MM-ddTHH:mm:ssZ")
$end = [DateTimeOffset]::UtcNow.AddHours(2).ToString("yyyy-MM-ddTHH:mm:ssZ")
$sum = "P10-review-$rnd"

# P10.1 event CRUD
$ce = Invoke-Api "A" POST "/calendar/events" @{ summary = $sum; location = "Room A"; description = "quarterly review"; start = $start; end = $end; reminder_minutes = 10 }
$eid = 0; if ($ce.Data) { $eid = $ce.Data.id }
$le = Invoke-Api "A" GET "/calendar/events"
$listed = $false
if ($le.Data) { foreach ($e in @($le.Data)) { if ("$($e.summary)" -eq $sum) { $listed = $true } } }
$nstart = [DateTimeOffset]::UtcNow.AddHours(3).ToString("yyyy-MM-ddTHH:mm:ssZ")
$ue = Invoke-Api "A" PUT "/calendar/events/$eid" @{ summary = $sum; location = "Room B"; start = $nstart }
$ueOk = ($ue.Status -in @(200,204))
$le2 = Invoke-Api "A" GET "/calendar/events"
$locUpdated = $false
if ($le2.Data) { foreach ($e in @($le2.Data)) { if ("$($e.summary)" -eq $sum -and "$($e.location)" -eq "Room B") { $locUpdated = $true } } }
Check P10.1a "event create/list/update" ($ce.Status -in @(200,201) -and $eid -gt 0 -and $listed -and $ueOk -and $locUpdated) "create=$($ce.Status) id=$eid listed=$listed update=$($ue.Status) locUpdated=$locUpdated"

# P10.2 shares
$sh = Invoke-Api "A" POST "/calendar/shares" @{ sharee_email = $P; read_only = $true }
$shl = Invoke-Api "A" GET "/calendar/shares"
$shareListed = $false
if ($shl.Data) { foreach ($s in @($shl.Data)) { if ("$($s.sharee_email)" -eq $P) { $shareListed = $true } } }
# shared user can list events (read) and cannot create (write) - record actual
$shid = 0
if ($shl.Data) { foreach ($s in @($shl.Data)) { if ("$($s.sharee_email)" -eq $P -and -not $shid) { $shid = $s.id } } }
$pb = Invoke-Api "B" GET "/calendar/events"
$pbStatus = $pb.Status
$pbCreate = Invoke-Api "B" POST "/calendar/events" @{ summary = "intruder-$rnd"; start = $start }
$sd = Invoke-Api "A" DELETE "/calendar/shares/$shid"
$shl2 = Invoke-Api "A" GET "/calendar/shares"
$shareGone = $true
if ($shl2.Data) { foreach ($s in @($shl2.Data)) { if ("$($s.sharee_email)" -eq $P) { $shareGone = $false } } }
Check P10.2 "share create/list/delete; shared read behavior recorded" ($sh.Status -in @(200,201) -and $shareListed -and $sd.Status -in @(200,204) -and $shareGone) "share=$($sh.Status) listed=$shareListed shareeList=$pbStatus shareeCreate=$($pbCreate.Status) del=$($sd.Status) gone=$shareGone"

# P10.3 signed ICS subscription link: public fetch, tamper rejected
$fd = Invoke-Api "A" GET "/calendar/feed"
$feedUrl = ""
if ($fd.Data -and $fd.Data.url) { $feedUrl = "$($fd.Data.url)" }
$icsOk = $false; $icsDetail = "feed=$($fd.Status) urlLen=$($feedUrl.Length)"
if ($feedUrl) {
    try {
        $ics = Invoke-WebRequest -Uri $feedUrl -UseBasicParsing -TimeoutSec 30
        $txt = "$($ics.Content)"
        $icsOk = ($ics.StatusCode -eq 200 -and $txt -match "BEGIN:VCALENDAR" -and $txt -match "BEGIN:VEVENT" -and $txt -match [regex]::Escape($sum))
        $icsDetail += " icsStatus=$($ics.StatusCode) ctype=$($ics.Headers['Content-Type']) vcalendar=$($txt -match 'BEGIN:VCALENDAR') vevent=$($txt -match 'BEGIN:VEVENT') summaryHit=$($txt -match [regex]::Escape($sum)) len=$($txt.Length)"
    } catch { $icsDetail += " fetchErr=$($_.Exception.Message)" }
}
$tampered = ""
if ($feedUrl) { $tampered = $feedUrl -replace "token=..", "token=XX" }
$tamStatus = 0
try { $tr = Invoke-WebRequest -Uri $tampered -UseBasicParsing -TimeoutSec 30; $tamStatus = [int]$tr.StatusCode } catch { if ($_.Exception.Response) { $tamStatus = [int]$_.Exception.Response.StatusCode } }
Check P10.3 "signed ICS feed: valid token serves VCALENDAR; tampered token 403" ($icsOk -and $tamStatus -in @(403,401)) "$icsDetail tampered=$tamStatus"

# P10.4 feed endpoint returns a URL
Check P10.4 "calendar/feed returns url" ($fd.Status -eq 200 -and $feedUrl -match "/calendar/export.ics\?email=") "status=$($fd.Status) hasUrl=$($feedUrl -ne '')"

# P10.5 boundary: invalid start rejected
$inv = Invoke-Api "A" POST "/calendar/events" @{ summary = "bad-$rnd"; start = "not-a-date" }
$invOk = ($inv.Status -eq 400)
Check P10.5 "invalid event start rejected 400" $invOk "status=$($inv.Status) body=$("$($inv.Body)".Trim())"

# cleanup: delete event
$de = Invoke-Api "A" DELETE "/calendar/events/$eid"
$le3 = Invoke-Api "A" GET "/calendar/events"
$evGone = $true
if ($le3.Data) { foreach ($e in @($le3.Data)) { if ("$($e.summary)" -eq $sum) { $evGone = $false } } }
"cleanup-note: eventDelete=$($de.Status) gone=$evGone"

"== auth401 self-heals: $script:Auth401 =="
$ud1 = Api $admin DELETE "/users/$U"
$ud2 = Api $admin DELETE "/users/$P"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P10"
"== done =="
