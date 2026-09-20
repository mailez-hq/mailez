# qa-tp-p7-drafts.ps1 - P7 drafts + scheduled send (TEST-PROMPTS.md P7), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P7 drafts/scheduled rev2 rnd=$rnd base=$Base =="
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

$U1 = "qa-p7a-$rnd@example.com"
$U2 = "qa-p7b-$rnd@example.com"
$admin = New-Session
foreach ($u in @($U1,$U2)) {
    $r = Api $admin POST "/users" @{ email = $u; password = "QaUser2026!"; display_name = "QA P7" }
    if ($r.Status -notin @(200,201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
Bind-Session "A" $U1 "QaUser2026!"
Bind-Session "B" $U2 "QaUser2026!"
"prep: users ready"

function Find-UidIn {
    param([string]$Who, [string]$Folder, [string]$Subject)
    $r = Invoke-Api $Who GET "/mail/messages?folder=$Folder&page=0"
    if ($r.Status -ne 200) { return 0 }
    foreach ($m in @($r.Data)) { if ($m.subject -eq $Subject) { return [uint32]$m.uid } }
    return 0
}
function Wait-Draft {
    param([string]$Who, [string]$Subject, [int]$Seconds)
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
        $u = Find-UidIn $Who "Drafts" $Subject
        if ($u -gt 0) { return $u }
        Start-Sleep -Seconds 4
    }
    return 0
}

# P7.1 draft lifecycle
$d1 = "P7-d1-$rnd"
$r1 = Invoke-Api "A" POST "/mail/draft" @{ subject = $d1; to = @($U2); text = "draft v1" }
$duid = 0; if ($r1.Data -and $r1.Data.uid) { $duid = [uint32]$r1.Data.uid }
$seen1 = Wait-Draft "A" $d1 90
$r2 = Invoke-Api "A" POST "/mail/draft" @{ subject = $d1; to = @($U2); text = "draft v2 merged"; replace_uid = $duid }
$duid2 = 0; if ($r2.Data -and $r2.Data.uid) { $duid2 = [uint32]$r2.Data.uid }
$md = Invoke-Api "A" GET "/mail/message?folder=Drafts&uid=$duid2"
$body2 = ""; if ($md.Data -and $md.Data.text_body) { $body2 = "$($md.Data.text_body)" }
$merged = ($body2 -match "v2 merged" -and $body2 -notmatch "draft v1")
$del = Invoke-Api "A" POST "/mail/delete" @{ folder = "Drafts"; uid = $duid2 }
$gone = $false
foreach ($i in 1..10) { Start-Sleep -Seconds 4; if ((Find-UidIn "A" "Drafts" $d1) -eq 0) { $gone = $true; break } }
Check P7.1 "draft save->list->edit merge->delete" ($r1.Status -eq 200 -and $seen1 -gt 0 -and $r2.Status -eq 200 -and $merged -and $del.Status -eq 204 -and $gone) "save=$($r1.Status) uid=$duid seen=$seen1 resave=$($r2.Status) uid2=$duid2 merged=$merged del=$($del.Status) gone=$gone"

# P7.2 Bcc regression: draft keeps to/cc/bcc
$d2 = "P7-d2-$rnd"
$rb = Invoke-Api "A" POST "/mail/draft" @{ subject = $d2; to = @($U2); cc = @("qa-p7cc-$rnd@example.com"); bcc = @("qa-p7bcc-$rnd@example.com"); text = "recipients" }
$buid = 0; if ($rb.Data -and $rb.Data.uid) { $buid = [uint32]$rb.Data.uid }
$seen2 = Wait-Draft "A" $d2 90
$mdb = Invoke-Api "A" GET "/mail/message?folder=Drafts&uid=$buid"
$toOk = $false; $ccOk = $false; $bccOk = $false
if ($mdb.Data) {
    if ($mdb.Data.to) { foreach ($a in @($mdb.Data.to)) { if ("$($a.email)" -eq $U2) { $toOk = $true } } }
    if ($mdb.Data.cc) { foreach ($a in @($mdb.Data.cc)) { if ("$($a.email)" -like "qa-p7cc-*") { $ccOk = $true } } }
    if ($mdb.Data.bcc) { foreach ($a in @($mdb.Data.bcc)) { if ("$($a.email)" -like "qa-p7bcc-*") { $bccOk = $true } } }
}
Check P7.2 "draft keeps To+Cc+Bcc (Bcc regression B8)" ($rb.Status -eq 200 -and $seen2 -gt 0 -and $toOk -and $ccOk -and $bccOk) "uid=$buid to=$toOk cc=$ccOk bcc=$bccOk"

# P7.3 draft with attachment
$d3 = "P7-d3-$rnd"
$aTxt = [Text.Encoding]::UTF8.GetBytes("p7 draft attachment")
$aB64 = [Convert]::ToBase64String($aTxt)
$ra = Invoke-Api "A" POST "/mail/draft" @{ subject = $d3; to = @($U2); text = "att draft"; attachments = @(@{ filename = "p7a.txt"; content_type = "text/plain"; size = $aTxt.Length; data = $aB64 }) }
$auid = 0; if ($ra.Data -and $ra.Data.uid) { $auid = [uint32]$ra.Data.uid }
$seen3 = Wait-Draft "A" $d3 90
$mda = Invoke-Api "A" GET "/mail/message?folder=Drafts&uid=$auid"
$attOk = $false; $attName = ""
if ($mda.Data -and $mda.Data.has_attachment -and $mda.Data.attachments) { $attOk = $true; $attName = "$($mda.Data.attachments[0].filename)" }
Check P7.3 "draft with attachment preserved on reopen" ($ra.Status -eq 200 -and $seen3 -gt 0 -and $attOk -and $attName -eq "p7a.txt") "uid=$auid hasAtt=$attOk name=$attName (send-side delivery covered by P3.4)"

# P7.4 scheduled auto-fire
$s4 = "P7-sched-$rnd"
$fire = [DateTimeOffset]::UtcNow.AddSeconds(75).ToString("yyyy-MM-ddTHH:mm:ssZ")
$rs = Invoke-Api "A" POST "/mail/send" @{ to = @($U2); subject = $s4; body = "scheduled fire"; send_at = $fire }
$oid = 0; if ($rs.Data -and $rs.Data.outbox_id) { $oid = [int64]$rs.Data.outbox_id }
$sl0 = Invoke-Api "A" GET "/mail/scheduled"
$listed = $false
if ($sl0.Data) { foreach ($e in @($sl0.Data)) { if ("$($e.subject)" -eq $s4) { $listed = $true } } }
$recv = 0
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 240) {
    $recv = Find-UidIn "B" "Inbox" $s4
    if ($recv -gt 0) { break }
    Start-Sleep -Seconds 6
}
$rcv = Invoke-Api "B" GET "/mail/message?folder=Inbox&uid=$recv"
$rcvBody = ""; if ($rcv.Data -and $rcv.Data.text_body) { $rcvBody = "$($rcv.Data.text_body)" }
$rcvOk = ($rcvBody -match "scheduled fire")
$sl1 = Invoke-Api "A" GET "/mail/scheduled"
$cleared = $true
if ($sl1.Data) { foreach ($e in @($sl1.Data)) { if ("$($e.subject)" -eq $s4) { $cleared = $false } } }
Check P7.4 "scheduled send fires on time, leaves outbox" ($rs.Status -eq 200 -and $oid -gt 0 -and $listed -and $recv -gt 0 -and $cleared -and $rcvOk) "bodyOk=$rcvOk sched=$($rs.Status) outboxId=$oid listed=$listed received(uid)=$recv in=$([int]$sw.Elapsed.TotalSeconds)s outboxCleared=$cleared"

# P7.5 + P7.6: two schedules coexist; cancel one; to-draft the other
$s5 = "P7-cancel-$rnd"
$s6 = "P7-todraft-$rnd"
$f5 = [DateTimeOffset]::UtcNow.AddSeconds(180).ToString("yyyy-MM-ddTHH:mm:ssZ")
[void](Invoke-Api "A" POST "/mail/send" @{ to = @($U2); subject = $s5; body = "to cancel"; send_at = $f5 })
[void](Invoke-Api "A" POST "/mail/send" @{ to = @($U2); subject = $s6; body = "to draft"; send_at = $f5 })
$sl2 = Invoke-Api "A" GET "/mail/scheduled"
$id5 = 0; $id6 = 0; $both = $false
if ($sl2.Data) {
    foreach ($e in @($sl2.Data)) {
        if ("$($e.subject)" -eq $s5) { $id5 = [int64]$e.id }
        if ("$($e.subject)" -eq $s6) { $id6 = [int64]$e.id }
    }
    $both = ($id5 -gt 0 -and $id6 -gt 0)
}
$cx = Invoke-Api "A" DELETE "/mail/outbox/$id5"
$sl3 = Invoke-Api "A" GET "/mail/scheduled"
$gone5 = $true
if ($sl3.Data) { foreach ($e in @($sl3.Data)) { if ("$($e.subject)" -eq $s5) { $gone5 = $false } } }
$td = Invoke-Api "A" POST "/mail/outbox/$id6/to-draft"
$tduid = 0; if ($td.Data -and $td.Data.uid) { $tduid = [uint32]$td.Data.uid }
$tdSeen = 0
if ($tduid -gt 0) { $tdSeen = Wait-Draft "A" $s6 90 }
$mtd = Invoke-Api "A" GET "/mail/message?folder=Drafts&uid=$tduid"
$tdBody = ""; if ($mtd.Data -and $mtd.Data.text_body) { $tdBody = "$($mtd.Data.text_body)" }
Check P7.5 "two schedules coexist; cancel removes entry" ($both -and $cx.Status -eq 204 -and $gone5) "ids=$id5/$id6 both=$both cancel=$($cx.Status) gone=$gone5"
Check P7.6 "to-draft restores content into Drafts" ($td.Status -eq 200 -and $tdSeen -gt 0 -and $tdBody -match "to draft") "resp=$($td.Status) uid=$tduid seen=$tdSeen bodyOk=$($tdBody -match 'to draft')"

# P7.7 boundaries
$rp = Invoke-Api "A" POST "/mail/send" @{ to = @($U2); subject = "past"; body = "x"; send_at = "2020-01-01T00:00:00Z" }
$ri = Invoke-Api "A" POST "/mail/send" @{ to = @($U2); subject = "badfmt"; body = "x"; send_at = "not-a-time" }
$okPast = ($rp.Status -eq 400) # wording "send_at must be in the future" verified at compose.go:127; Api helper does not surface error bodies
$okBad = ($ri.Status -eq 400)
Check P7.7 "past send_at and invalid format rejected 400" ($okPast -and $okBad) "past=$($rp.Status)/$okPast badfmt=$($ri.Status)/$okBad bodies=[$($rp.Body)][$($ri.Body)]"

"== auth401 self-heals: $script:Auth401 =="
$ud1 = Api $admin DELETE "/users/$U1"
$ud2 = Api $admin DELETE "/users/$U2"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P7"
"== done =="


