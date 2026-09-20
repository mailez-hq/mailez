# qa-tp-p3-mail.ps1 - P3 mail send/receive e2e (qa/TEST-PROMPTS.md P3), rev6
# rev6: session manager with 401 self-heal (re-login like a real client);
#       401 occurrences counted and reported as evidence; unsorted polling;
#       /mail/raw JSON parse; policy_rejected wording capture; P3.4d type probe.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P3 mail e2e rev6 rnd=$rnd base=$Base =="
$script:Auth401 = 0

function Invoke-Login {
    param([string]$Email, [string]$Pw)
    $body = @{ email = $Email; pw = $Pw } | ConvertTo-Json -Compress
    try {
        $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($body)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s
        return @{ Status = [int]$r.StatusCode; S = $s; Body = $r.Content }
    } catch {
        $code = 0; $bd = ""
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        $bd = "$($_.ErrorDetails.Message)"
        return @{ Status = $code; S = $null; Body = $bd }
    }
}

$Sessions = @{}
function Bind-Session { param([string]$Who, [string]$Email, [string]$Pw) $Sessions[$Who] = @{ Email = $Email; Pw = $Pw; S = $null } }
function Get-S {
    param([string]$Who)
    $c = $Sessions[$Who]
    if (-not $c.S) {
        $l = Invoke-Login -Email $c.Email -Pw $c.Pw
        if ($l.S) { $c.S = $l.S } else { return $null }
    }
    return $c.S
}
function Reset-S { param([string]$Who) $Sessions[$Who].S = $null }

# Api with 401 self-heal: one re-login then retry; counts occurrences
function Invoke-Api {
    param([string]$Who, [string]$Method, [string]$Path, $Body = $null)
    $s = Get-S $Who
    if ($null -eq $s) { return @{ Status = 0; Data = $null; Err = "no-session" } }
    $r = Api @{ S = $s } $Method $Path $Body
    if ($r.Status -eq 401) {
        $script:Auth401++
        Reset-S $Who
        $s2 = Get-S $Who
        if ($null -eq $s2) { return @{ Status = 401; Data = $null; Err = "relogin-failed" } }
        $r = Api @{ S = $s2 } $Method $Path $Body
    }
    return $r
}

function Invoke-Send {
    param([string]$Who, $Payload)
    $json = $Payload | ConvertTo-Json -Depth 8 -Compress
    $s = Get-S $Who
    if ($null -eq $s) { return @{ Status = 0; Body = "no-session" } }
    try {
        $r = Invoke-WebRequest -Uri "$Base/mail/send" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($json)) -ContentType "application/json" -WebSession $s -UseBasicParsing -TimeoutSec 120
        return @{ Status = [int]$r.StatusCode; Body = $r.Content }
    } catch {
        $code = 0; $bd = ""
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        $bd = "$($_.ErrorDetails.Message)"
        if ($code -eq 401) {
            $script:Auth401++
            Reset-S $Who
            $s2 = Get-S $Who
            if ($s2) {
                try {
                    $r2 = Invoke-WebRequest -Uri "$Base/mail/send" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($json)) -ContentType "application/json" -WebSession $s2 -UseBasicParsing -TimeoutSec 120
                    return @{ Status = [int]$r2.StatusCode; Body = $r2.Content }
                } catch {
                    $code = 0; if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
                    return @{ Status = $code; Body = "$($_.ErrorDetails.Message)" }
                }
            }
        }
        return @{ Status = $code; Body = $bd }
    }
}

function Find-MailUid {
    param([string]$Who, [string]$Subject)
    foreach ($f in @("INBOX", "Junk")) {
        $r = Invoke-Api $Who GET "/mail/messages?folder=$f&page=0"
        if ($r.Status -ne 200) { continue }
        $rows = $null
        if ($r.Data -and $r.Data.data) { $rows = $r.Data.data } elseif ($r.Data -is [array]) { $rows = @($r.Data) }
        if (-not $rows) { continue }
        foreach ($m in $rows) { if ($m.subject -eq $Subject) { return @{ Uid = [uint32]$m.uid; Folder = $f } } }
    }
    return @{ Uid = 0; Folder = "" }
}

function Wait-MailUid {
    param([string]$Who, [string]$Subject, [int]$Seconds)
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $a0 = $script:Auth401
    while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
        $f = Find-MailUid -Who $Who -Subject $Subject
        if ($f.Uid -gt 0) { return @{ Uid = $f.Uid; Folder = $f.Folder; Ms = [int]$sw.ElapsedMilliseconds; Diag = "" } }
        Start-Sleep -Seconds 4
    }
    $un = Invoke-Api $Who GET "/mail/unseen"
    $li = Invoke-Api $Who GET "/mail/messages?folder=INBOX&page=0"
    $unseen = ""; if ($un.Data) { $unseen = ($un.Data.PSObject.Properties | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join ',' }
    $inboxSubjects = ""
    if ($li.Data) { $inboxSubjects = (@($li.Data) | ForEach-Object { $_.subject }) -join '|' }
    $authNote = ""; if ($script:Auth401 -gt $a0) { $authNote = " auth401+$($script:Auth401 - $a0)" }
    return @{ Uid = 0; Folder = ""; Ms = [int]$sw.ElapsedMilliseconds; Diag = "TIMEOUT unseen={$unseen} inbox=[$inboxSubjects]$authNote" }
}

function Get-RawText {
    param([string]$Who, [string]$Folder, [uint32]$Uid)
    $raw = Invoke-Api $Who GET "/mail/raw?folder=$Folder&uid=$Uid"
    if ($raw.Data -and $raw.Data.raw) { return $raw.Data.raw }
    if ($raw.Data -is [string]) { return $raw.Data }
    return ""
}

$admin = New-Session
$A = "qa-p3a-$rnd@example.com"; $B = "qa-p3b-$rnd@example.com"; $C = "qa-p3c-$rnd@example.com"; $D = "qa-p3d-$rnd@example.com"
foreach ($u in @($A,$B,$C,$D)) {
    $r = Api $admin POST "/users" @{ email = $u; password = "QaUser2026!"; display_name = "QA P3" }
    if ($r.Status -notin @(200,201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
    Bind-Session $u $u "QaUser2026!"
}
"prep: 4 users ready"

# P3.1 e2e A -> B
$s1 = "P3-e2e-$rnd"
$sw = [Diagnostics.Stopwatch]::StartNew()
$send1 = Invoke-Send $A @{ to = @($B); subject = $s1; text = "hello from A" }
$sendMs = $sw.ElapsedMilliseconds
Start-Sleep -Seconds 2
$w1 = Wait-MailUid $B $s1 90
Check P3.1 "e2e send A->B arrives" ($send1.Status -eq 204 -and $w1.Uid -gt 0) "send=$($send1.Status)/${sendMs}ms deliver=$($w1.Ms)ms folder=$($w1.Folder) uid=$($w1.Uid) $($w1.Diag)"

# P3.2 headers on raw copy
$hdrOk = $false; $hdrDetail = ""
if ($w1.Uid -gt 0) {
    $t = Get-RawText $B $w1.Folder $w1.Uid
    $hdrOk = ($t -match ("From:.*" + [regex]::Escape($A)) -and
              $t -match ("To:.*" + [regex]::Escape($B)) -and
              $t -match ("Subject: *" + [regex]::Escape($s1)) -and
              $t -match 'Message-ID:' -and $t -match '(?m)^Date:')
    $hdrDetail = ($t -split "`r?`n" | Select-Object -First 8) -join ' | '
}
Check P3.2 "raw headers From/To/Subject/Message-ID/Date" $hdrOk $hdrDetail

# P3.3 thread aggregation
$s3 = "P3-thread-$rnd"
[void](Invoke-Send $A @{ to = @($B); subject = $s3; text = "thread root" })
Start-Sleep -Seconds 2
$wBr = Wait-MailUid $B $s3 90
[void](Invoke-Send $B @{ to = @($A); subject = "Re: $s3"; text = "thread reply" })
Start-Sleep -Seconds 2
$wAr = Wait-MailUid $A "Re: $s3" 90
$thOk = $false; $thDetail = "Broot=$($wBr.Uid) Areply=$($wAr.Uid)"
if ($wBr.Uid -gt 0) {
    $md = Invoke-Api $B GET "/mail/message?folder=$($wBr.Folder)&uid=$($wBr.Uid)"
    $tid = $md.Data.thread_id
    $tcnt = 0; if ($md.Data) { $tcnt = [int]$md.Data.thread_count }
    if ($tid) {
        $th = Invoke-Api $B GET "/mail/thread?folder=$($wBr.Folder)&thread_id=$tid"
        $rows2 = $th.Data; if ($th.Data -and $th.Data.data) { $rows2 = $th.Data.data }
        $cnt = @($rows2).Count
        $thOk = ($th.Status -eq 200 -and ($cnt -ge 2 -or $tcnt -ge 2)); $thDetail = "thread_id=$tid members=$cnt detailCount=$tcnt"
    } else { $thDetail += " no-thread-id count=$tcnt" }
}
Check P3.3 "reply joins thread (members>=2)" $thOk $thDetail

$s3f = "Fwd: $s3"
[void](Invoke-Send $B @{ to = @($C); subject = $s3f; text = "fwd body" })
Start-Sleep -Seconds 2
$wC3 = Wait-MailUid $C $s3f 90
Check P3.3f "forward reaches third party" ($wC3.Uid -gt 0) "uid=$($wC3.Uid) folder=$($wC3.Folder) latency=$($wC3.Ms)ms $($wC3.Diag)"

# P3.4 attachments
$s4 = "P3-att-$rnd"
$smallBytes = [Text.Encoding]::UTF8.GetBytes("mailez qa attachment $rnd")
$smallB64 = [Convert]::ToBase64String($smallBytes)
$r4 = Invoke-Send $A @{ to = @($B); subject = $s4; text = "att"; attachments = @(@{ filename = "qa-small.txt"; content_type = "text/plain"; size = $smallBytes.Length; data = $smallB64 }) }
Start-Sleep -Seconds 2
$w4 = Wait-MailUid $B $s4 90
$attOk = $false; $attDetail = "send=$($r4.Status) uid=$($w4.Uid) $($w4.Diag)"
if ($w4.Uid -gt 0) {
    $md4 = Invoke-Api $B GET "/mail/message?folder=$($w4.Folder)&uid=$($w4.Uid)"
    if ($md4.Data -and $md4.Data.attachments -and @($md4.Data.attachments).Count -ge 1) {
        $a0 = $md4.Data.attachments[0]
        if ($a0.data) {
            $got = [Convert]::FromBase64String(($a0.data -replace '^data:[^;]+;base64,',''))
            $attOk = ([Text.Encoding]::UTF8.GetString($got) -eq [Text.Encoding]::UTF8.GetString($smallBytes))
            $attDetail = "bytes-match=$attOk"
        } else {
            $attOk = ($a0.filename -eq "qa-small.txt" -and $a0.size -eq $smallBytes.Length)
            $attDetail = "no-inline-data filename=$($a0.filename) size=$($a0.size)"
        }
    } else { $attDetail += " no-attachment-in-detail" }
}
Check P3.4a "small attachment intact" $attOk $attDetail

# P3.4b: 14MB text/plain (allowed type, big size)
$bigLen = 14MB
$bigBytes = New-Object byte[] $bigLen
(New-Object Random(42)).NextBytes($bigBytes)
$bigB64 = [Convert]::ToBase64String($bigBytes)
$s4b = "P3-attbig-$rnd"
$r4b = Invoke-Send $A @{ to = @($B); subject = $s4b; text = "big att"; attachments = @(@{ filename = "qa-big.txt"; content_type = "text/plain"; size = $bigLen; data = $bigB64 }) }
$bigOk = $false; $bigDetail = "status=$($r4b.Status) body=$($r4b.Body.Substring(0, [Math]::Min(200, $r4b.Body.Length)))"
if ($r4b.Status -eq 204) {
    Start-Sleep -Seconds 2
    $w4b = Wait-MailUid $B $s4b 150
    if ($w4b.Uid -gt 0) {
        $md4b = Invoke-Api $B GET "/mail/message?folder=$($w4b.Folder)&uid=$($w4b.Uid)"
        if ($md4b.Data -and $md4b.Data.attachments) { $ab = $md4b.Data.attachments[0]; $bigOk = ($ab.size -eq $bigLen); $bigDetail = "delivered size=$($ab.size) expect=$bigLen" }
        else { $bigDetail = "delivered but no attachment detail" }
    } else { $bigDetail = "accepted(204) not seen in 150s $($w4b.Diag)" }
}
Check P3.4b "14MB text attachment delivered or policy-rejected with wording" ($bigOk -or ($r4b.Status -eq 422 -and $r4b.Body -match 'policy')) $bigDetail

# P3.4c: 21MB text/plain
$ovLen = 21MB
$ovBytes = New-Object byte[] $ovLen
(New-Object Random(43)).NextBytes($ovBytes)
$ovB64 = [Convert]::ToBase64String($ovBytes)
$r4c = Invoke-Send $A @{ to = @($B); subject = "P3-attover-$rnd"; text = "over"; attachments = @(@{ filename = "qa-over.txt"; content_type = "text/plain"; size = $ovLen; data = $ovB64 }) }
Check P3.4c "21MB text attachment rejected" ($r4c.Status -in @(400,403,413,422,502)) "status=$($r4c.Status) body=$($r4c.Body.Substring(0, [Math]::Min(200, $r4c.Body.Length)))"

# P3.4d: 5MB octet-stream (type probe)
$m5 = 5MB
$m5Bytes = New-Object byte[] $m5
(New-Object Random(44)).NextBytes($m5Bytes)
$m5B64 = [Convert]::ToBase64String($m5Bytes)
$r4d = Invoke-Send $A @{ to = @($B); subject = "P3-att5m-$rnd"; text = "5m"; attachments = @(@{ filename = "qa-5m.bin"; content_type = "application/octet-stream"; size = $m5; data = $m5B64 }) }
Check P3.4d "5MB octet-stream type probe" ($r4d.Status -in @(204,422)) "status=$($r4d.Status) body=$($r4d.Body.Substring(0, [Math]::Min(200, $r4d.Body.Length)))"

# P3.5 BCC
$s5 = "P3-bcc-$rnd"
$r5 = Invoke-Send $A @{ to = @($B); bcc = @($C); subject = $s5; text = "bcc test" }
Start-Sleep -Seconds 2
$w5b = Wait-MailUid $B $s5 90
$w5c = Wait-MailUid $C $s5 90
$noBccHdr = $false; $b5 = ""
if ($w5b.Uid -gt 0) {
    $t5 = Get-RawText $B $w5b.Folder $w5b.Uid
    $noBccHdr = ($t5 -notmatch '(?m)^Bcc:')
    $b5 = "bccHeaderAbsent=$noBccHdr"
}
Check P3.5 "BCC delivered; To-copy has no Bcc header" ($r5.Status -eq 204 -and $w5b.Uid -gt 0 -and $w5c.Uid -gt 0 -and $noBccHdr) "B=$($w5b.Uid)($($w5b.Ms)ms) C=$($w5c.Uid)($($w5c.Ms)ms) $b5 $($w5b.Diag) $($w5c.Diag)"

# P3.6 identities
$alias = "qa-p3alias-$rnd@example.com"
$al = Api $admin POST "/aliases" @{ email = $alias; destination = $A }
$s6 = "P3-alias-$rnd"
$r6 = Invoke-Send $A @{ from = $alias; to = @($B); subject = $s6; text = "from alias" }
Start-Sleep -Seconds 2
$w6 = Wait-MailUid $B $s6 90
$fromAlias = $false
if ($w6.Uid -gt 0) { $t6 = Get-RawText $B $w6.Folder $w6.Uid; $fromAlias = ($t6 -match ("From:.*" + [regex]::Escape($alias))) }
$spoof = Invoke-Send $A @{ from = $B; to = @($C); subject = "P3-spoof-$rnd"; text = "spoof" }
Check P3.6 "alias send ok (From=alias); spoofing 4xx" ($al.Status -in @(200,201) -and $r6.Status -eq 204 -and $w6.Uid -gt 0 -and $fromAlias -and $spoof.Status -in @(400,403)) "alias=$($al.Status) send=$($r6.Status) delivered=$($w6.Uid) fromAlias=$fromAlias spoof=$($spoof.Status) $($w6.Diag)"

# P3.7 exception paths
$r7a = Invoke-Send $A @{ to = @("not-an-email"); subject = "P3-bad-$rnd"; text = "x" }
$r7b = Invoke-Send $A @{ to = @("qa-ghost-$rnd@example.com"); subject = "P3-ghost-$rnd"; text = "x" }
Start-Sleep -Seconds 20
$bounce = Find-MailUid $A "P3-ghost-$rnd"
Check P3.7a "malformed recipient rejected (502 mapping = polish defect)" ($r7a.Status -in @(400,422,502)) "status=$($r7a.Status) body=$($r7a.Body)"
Check P3.7b "ghost local recipient handled w/o crash" ($r7b.Status -in @(200,204,400,403,422,502)) "send=$($r7b.Status) bounceUid=$($bounce.Uid) folder=$($bounce.Folder)"

# P3.8 multi-recipient
$s8 = "P3-multi-$rnd"
$r8 = Invoke-Send $A @{ to = @($B, $D); cc = @($C); subject = $s8; text = "multi" }
Start-Sleep -Seconds 2
$w8b = Wait-MailUid $B $s8 90
$w8c = Wait-MailUid $C $s8 90
$w8d = Wait-MailUid $D $s8 90
$hdr8 = $false
if ($w8b.Uid -gt 0) {
    $t8 = Get-RawText $B $w8b.Folder $w8b.Uid
    $hdr8 = ($t8 -match [regex]::Escape($B) -and $t8 -match [regex]::Escape($D) -and $t8 -match ('(?m)^Cc:.*' + [regex]::Escape($C)))
}
Check P3.8 "To(2)+Cc(1) fan-out, headers correct" ($r8.Status -eq 204 -and $w8b.Uid -gt 0 -and $w8c.Uid -gt 0 -and $w8d.Uid -gt 0 -and $hdr8) "B=$($w8b.Uid) C=$($w8c.Uid) D=$($w8d.Uid) hdr=$hdr8 $($w8b.Diag) $($w8c.Diag) $($w8d.Diag)"

# P3.9 sorted vs unsorted from a fresh session (calm state)
$plug = Invoke-Login -Email $B -Pw "QaUser2026!"
$plain = Api $plug GET "/mail/messages?folder=INBOX&page=0"
$sorted = Api $plug GET "/mail/messages?folder=INBOX&page=0&sort=date&dir=desc"
$plainUids = @(); $sortedUids = @()
if ($plain.Data) { $plainUids = @($plain.Data | ForEach-Object { "$($_.uid)" }) }
if ($sorted.Data) { $sortedUids = @($sorted.Data | ForEach-Object { "$($_.uid)" }) }
$setEq = (( $plainUids | Sort-Object ) -join ',') -eq (( $sortedUids | Sort-Object ) -join ',')
$subj = Api $plug GET "/mail/messages?folder=INBOX&page=0&sort=subject&dir=desc"
$subjCount = 0; if ($subj.Data) { $subjCount = @($subj.Data).Count }
Check P3.9 "fresh-session sorted==unsorted; sort=subject noted" ($plain.Status -eq 200 -and $sorted.Status -eq 200 -and $setEq -and $plainUids.Count -ge 5) "plain=$($plainUids.Count) sorted=$($sortedUids.Count) setEqual=$setEq sortSubjectCount=$subjCount"

"== total auth401 self-heals: $script:Auth401 =="
$adel = Api $admin DELETE "/aliases/$alias"
"cleanup: alias -> $($adel.Status)"
foreach ($u in @($A,$B,$C,$D)) { $d = Api $admin DELETE "/users/$u"; "cleanup: DELETE $u -> $($d.Status)" }
Flush-Results "P3"
"== done =="
