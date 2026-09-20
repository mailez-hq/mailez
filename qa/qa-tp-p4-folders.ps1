# qa-tp-p4-folders.ps1 - P4 folder/flag/label/zip/concurrency (TEST-PROMPTS.md P4), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P4 folders/flags/labels/zip rev1 rnd=$rnd base=$Base =="

function Invoke-Login {
    param([string]$Email, [string]$Pw)
    $body = @{ email = $Email; pw = $Pw } | ConvertTo-Json -Compress
    try {
        $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($body)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s
        return @{ Status = [int]$r.StatusCode; S = $s }
    } catch {
        $code = 0; if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ Status = $code; S = $null }
    }
}

$U = "qa-p4a-$rnd@example.com"
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
$admin = New-Session
$r = Api $admin POST "/users" @{ email = $U; password = "QaUser2026!"; display_name = "QA P4" }
if ($r.Status -notin @(200,201)) { "prep FAIL create -> $($r.Status)"; exit 1 }
Bind-Session "U1" $U "QaUser2026!"
"prep: user ready (create=$($r.Status))"

function List-Folders {
    param([string]$Who)
    $r = Invoke-Api $Who GET "/mail/folders"
    if ($r.Status -ne 200) { return @() }
    return @($r.Data | ForEach-Object { "$_" })
}
function Find-Uid {
    param([string]$Who, [string]$Folder, [string]$Subject)
    $r = Invoke-Api $Who GET "/mail/messages?folder=$Folder&page=0"
    if ($r.Status -ne 200) { return 0 }
    $rows = @($r.Data)
    foreach ($m in $rows) { if ($m.subject -eq $Subject) { return [uint32]$m.uid } }
    return 0
}
function Wait-Subject {
    param([string]$Who, [string]$Subject, [int]$Seconds)
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
        $u = Find-Uid $Who "Inbox" $Subject
        if ($u -gt 0) { return $u }
        Start-Sleep -Seconds 4
    }
    $li = Invoke-Api $Who GET "/mail/messages?folder=Inbox&page=0"
    $subs = ""; if ($li.Data) { $subs = (@($li.Data) | ForEach-Object { $_.subject }) -join '|' }
    return 0
}
function Send-Self {
    param([string]$Who, [string]$Subject, [string]$Text)
    $r = Invoke-Api $Who POST "/mail/send" @{ to = @($U); subject = $Subject; text = $Text }
    return $r.Status
}

# P4.1 system folders
$need = @("Inbox","Sent","Drafts","Trash","Junk")
$ok41 = $false; $d41 = ""
foreach ($i in 1..15) {
    Start-Sleep -Seconds 4
    $f0 = List-Folders "U1"
    $missing = @()
    foreach ($n in $need) { $hit = $false; foreach ($x in $f0) { if ($x -ieq $n) { $hit = $true } }; if (-not $hit) { $missing += $n } }
    if ($f0.Count -ge 5 -and $missing.Count -eq 0) { $ok41 = $true; $d41 = "folders=[$($f0 -join ',')] ready-after=~$($i*4)s"; break }
    $d41 = "folders=[$($f0 -join ',')] missing=[$($missing -join ',')] after=$($i*4)s"
}
Check P4.1 "folders list has 5 system folders (bootstrap window)" $ok41 $d41

# P4.2 create folder
$fold = "qa4f-$rnd"
$c1 = Invoke-Api "U1" POST "/mail/folders" @{ name = $fold }
$ok2 = $false; $d2 = "create=$($c1.Status)"
foreach ($i in 1..15) { Start-Sleep -Seconds 4; $f1 = List-Folders "U1"; $hit = $false; foreach ($f in $f1) { if ($f -ieq $fold) { $hit = $true } }; if ($hit) { $ok2 = $true; $d2 += " appeared-after=$($i*4)s"; break } }
Check P4.2 "create custom folder" $ok2 $d2

# P4.3 rename folder
$foldR = "qa4r-$rnd"
$c2 = Invoke-Api "U1" PUT "/mail/folders" @{ name = $fold; new_name = $foldR }
$ok3 = $false; $d3 = "rename=$($c2.Status)"
foreach ($i in 1..15) { Start-Sleep -Seconds 4; $f2 = List-Folders "U1"; $new = $false; $old = $false; foreach ($f in $f2) { if ($f -ieq $foldR) { $new = $true }; if ($f -ieq $fold) { $old = $true } }; if ($new -and -not $old) { $ok3 = $true; $d3 += " renamed-after=$($i*4)s"; break } }
Check P4.3 "rename custom folder" $ok3 $d3

# P4.13 rename system folder must 400
$neg = Invoke-Api "U1" PUT "/mail/folders" @{ name = "Inbox"; new_name = "InboxX" }
Check P4.13 "rename system folder rejected 400" ($neg.Status -eq 400) "status=$($neg.Status) body=$($neg.Body)"

# seed 3 messages
$m1 = "P4-m1-$rnd"; $m2 = "P4-m2-$rnd"; $m3 = "P4-m3-$rnd"
[void](Send-Self "U1" $m1 "body one")
[void](Send-Self "U1" $m2 "body two")
[void](Send-Self "U1" $m3 "body three")
$u1 = Wait-Subject "U1" $m1 150
$u2 = Wait-Subject "U1" $m2 150
$u3 = Wait-Subject "U1" $m3 150
"seed: uids m1=$u1 m2=$u2 m3=$u3"
$seedLi = Invoke-Api "U1" GET "/mail/unseen"

# P4.7 unseen then read-all
$un0 = Invoke-Api "U1" GET "/mail/unseen"
$inboxUn0 = 0
if ($un0.Data) { foreach ($p in $un0.Data.PSObject.Properties) { if ($p.Name -ieq "Inbox") { $inboxUn0 = [int]$p.Value } } }
$ra = Invoke-Api "U1" POST "/mail/read-all" @{ folder = "Inbox" }
$inboxUn1 = -1
foreach ($i in 1..10) { Start-Sleep -Seconds 3; $un1 = Invoke-Api "U1" GET "/mail/unseen"; $inboxUn1 = 0; if ($un1.Data) { foreach ($p in $un1.Data.PSObject.Properties) { if ($p.Name -ieq "Inbox") { $inboxUn1 = [int]$p.Value } } }; if ($inboxUn1 -eq 0) { break } }
Check P4.7 "unseen>0 -> read-all -> unseen=0" ($seedLi.Status -eq 200 -and $inboxUn0 -ge 3 -and $ra.Status -eq 204 -and $inboxUn1 -eq 0) "unseenBefore=$inboxUn0 readAll=$($ra.Status) unseenAfter=$inboxUn1"

# P4.6 flags: seen + flagged on m2
$fSeen = Invoke-Api "U1" POST "/mail/flag" @{ folder = "Inbox"; uid = $u2; flag = "seen"; value = $true }
$fFlag = Invoke-Api "U1" POST "/mail/flag" @{ folder = "Inbox"; uid = $u2; flag = "flagged"; value = $true }
$md2 = Invoke-Api "U1" GET "/mail/message?folder=Inbox&uid=$u2"
$flags2 = @(); if ($md2.Data -and $md2.Data.flags) { $flags2 = @($md2.Data.flags | ForEach-Object { "$_" }) }
$hasSeen = $false; $hasFlag = $false
foreach ($x in $flags2) { if ($x -imatch '^\[?\\?seen\]?$' -or $x -ieq 'Seen') { $hasSeen = $true }; if ($x -imatch 'flagged' -or $x -ieq 'Flagged') { $hasFlag = $true } }
$fUnflag = Invoke-Api "U1" POST "/mail/flag" @{ folder = "Inbox"; uid = $u2; flag = "flagged"; value = $false }
$md2b = Invoke-Api "U1" GET "/mail/message?folder=Inbox&uid=$u2"
$flags2b = @(); if ($md2b.Data -and $md2b.Data.flags) { $flags2b = @($md2b.Data.flags | ForEach-Object { "$_" }) }
$hasFlag2 = $false; foreach ($x in $flags2b) { if ($x -imatch 'flagged' -or $x -ieq 'Flagged') { $hasFlag2 = $true } }
Check P4.6 "flag seen/flagged set+clear reflect in detail" ($fSeen.Status -eq 204 -and $fFlag.Status -eq 204 -and $hasSeen -and $hasFlag -and $fUnflag.Status -eq 204 -and -not $hasFlag2) "seen=$($fSeen.Status)/$hasSeen flagged=$($fFlag.Status)/$hasFlag unflag=$($fUnflag.Status)/cleared=$(-not $hasFlag2) flags=[$($flags2 -join ',')]"

# P4.5 move m1 -> renamed folder
$mv = Invoke-Api "U1" POST "/mail/move" @{ folder = "Inbox"; uids = @($u1); destination = $foldR }
$inSrc = -1; $inDst = -1
foreach ($i in 1..15) {
    Start-Sleep -Seconds 4
    $ls = Invoke-Api "U1" GET "/mail/messages?folder=Inbox&page=0"
    $ld = Invoke-Api "U1" GET "/mail/messages?folder=$foldR&page=0"
    $inSrc = 0; $inDst = 0
    if ($ls.Data) { foreach ($m in @($ls.Data)) { if ($m.subject -eq $m1) { $inSrc = 1 } } }
    if ($ld.Data) { foreach ($m in @($ld.Data)) { if ($m.subject -eq $m1) { $inDst = 1 } } }
    if ($inSrc -eq 0 -and $inDst -eq 1) { break }
}
Check P4.5 "move Inbox->custom folder" ($mv.Status -eq 204 -and $inSrc -eq 0 -and $inDst -eq 1) "move=$($mv.Status) srcHas=$inSrc dstHas=$inDst"

# P4.4 clear folder (empties it)
$cl = Invoke-Api "U1" POST "/mail/folders/clear" @{ name = $foldR }
$cleared = $false
foreach ($i in 1..10) { Start-Sleep -Seconds 4; $lc = Invoke-Api "U1" GET "/mail/messages?folder=$foldR&page=0"; $cnt = 0; if ($lc.Data) { $cnt = @($lc.Data).Count }; if ($cnt -eq 0) { $cleared = $true; break } }
Check P4.4 "clear folder empties it" ($cl.Status -eq 204 -and $cleared) "clear=$($cl.Status) emptied=$cleared"

# P4.8 delete m2: Inbox -> Trash -> purge
$dl = Invoke-Api "U1" POST "/mail/delete" @{ folder = "Inbox"; uid = $u2 }
$inInbox = -1; $inTrash = -1
foreach ($i in 1..15) {
    Start-Sleep -Seconds 4
    $li2 = Invoke-Api "U1" GET "/mail/messages?folder=Inbox&page=0"
    $lt2 = Invoke-Api "U1" GET "/mail/messages?folder=Trash&page=0"
    $inInbox = 0; $inTrash = 0
    if ($li2.Data) { foreach ($m in @($li2.Data)) { if ($m.subject -eq $m2) { $inInbox = 1 } } }
    if ($lt2.Data) { foreach ($m in @($lt2.Data)) { if ($m.subject -eq $m2) { $inTrash = 1 } } }
    if ($inInbox -eq 0 -and $inTrash -eq 1) { break }
}
$tu2 = Find-Uid "U1" "Trash" $m2
$dl2 = Invoke-Api "U1" POST "/mail/delete" @{ folder = "Trash"; uid = $tu2 }
$purged = $false
foreach ($i in 1..10) { Start-Sleep -Seconds 4; $lt3 = Invoke-Api "U1" GET "/mail/messages?folder=Trash&page=0"; $hit = 0; if ($lt3.Data) { foreach ($m in @($lt3.Data)) { if ($m.subject -eq $m2) { $hit = 1 } } }; if ($hit -eq 0) { $purged = $true; break } }
Check P4.8 "delete -> Trash; delete in Trash -> purged" ($dl.Status -eq 204 -and $inInbox -eq 0 -and $inTrash -eq 1 -and $dl2.Status -eq 204 -and $purged) "del=$($dl.Status) inbox=$inInbox trash=$inTrash purge=$($dl2.Status) gone=$purged"

# P4.9 labels lifecycle on m3
$lab = "qa4L-$rnd"
$lbNew = Invoke-Api "U1" POST "/mail/labels" @{ name = $lab; color = "#3366cc" }
$lbList = Invoke-Api "U1" GET "/mail/labels"
$listed = $false
if ($lbList.Data) { foreach ($x in @($lbList.Data)) { if ("$($x.name)" -eq $lab) { $listed = $true } } }
$tagOn = Invoke-Api "U1" POST "/mail/flag" @{ folder = "Inbox"; uid = $u3; flag = $lab; value = $true }
$md3 = Invoke-Api "U1" GET "/mail/message?folder=Inbox&uid=$u3"
$tagSeen = $false
if ($md3.Data -and $md3.Data.flags) { foreach ($x in @($md3.Data.flags)) { if ("$x" -eq $lab) { $tagSeen = $true } } }
$srch = Invoke-Api "U1" GET ("/mail/search?folder=Inbox&q=" + [uri]::EscapeDataString("label:$lab"))
$srchHit = $false
if ($srch.Data) { foreach ($m in @($srch.Data)) { if ($m.subject -eq $m3) { $srchHit = $true } } }
$tagOff = Invoke-Api "U1" POST "/mail/flag" @{ folder = "Inbox"; uid = $u3; flag = $lab; value = $false }
$srch2 = Invoke-Api "U1" GET ("/mail/search?folder=Inbox&q=" + [uri]::EscapeDataString("label:$lab"))
$srchGone = $true
if ($srch2.Data) { foreach ($m in @($srch2.Data)) { if ($m.subject -eq $m3) { $srchGone = $false } } }
$lab2 = "qa4L2-$rnd"
$lbRen = Invoke-Api "U1" POST "/mail/labels/rename" @{ from = $lab; to = $lab2 }
$lbDel = Invoke-Api "U1" DELETE "/mail/labels?name=$lab2"
$lbList2 = Invoke-Api "U1" GET "/mail/labels"
$deleted = $true
if ($lbList2.Data) { foreach ($x in @($lbList2.Data)) { if ("$($x.name)" -eq $lab2) { $deleted = $false } } }
Check P4.9 "labels: create/tag/search/untag/rename/delete" ($lbNew.Status -in @(200,201,204) -and $listed -and $tagOn.Status -eq 204 -and $tagSeen -and $srchHit -and $tagOff.Status -eq 204 -and $srchGone -and $lbRen.Status -in @(200,201,204) -and $lbDel.Status -eq 204 -and $deleted) "create=$($lbNew.Status) listed=$listed tag=$($tagOn.Status)/$tagSeen searchHit=$srchHit untag/$srchGone rename=$($lbRen.Status) del=$($lbDel.Status)/$deleted"

# P4.10 pagination + sort
foreach ($i in 4..8) {
    $txt = "x" * (200 * $i)
    [void](Send-Self "U1" "P4-m$i-$rnd" $txt)
    Start-Sleep -Seconds 2
}
$p0ok = $false; $p1ok = $false; $sortOk = $false; $d10 = ""
foreach ($i in 1..40) {
    Start-Sleep -Seconds 4
    $a = Invoke-Api "U1" GET "/mail/messages?folder=Inbox&page=0"
    $b = Invoke-Api "U1" GET "/mail/messages?folder=Inbox&page=1"
    $ua0 = @(); $ub0 = @()
    if ($a.Data) { $ua0 = @($a.Data | ForEach-Object { [uint32]$_.uid }) }
    if ($b.Data) { $ub0 = @($b.Data | ForEach-Object { [uint32]$_.uid }) }
    if ($ua0.Count -ge 6 -and $ub0.Count -eq 0) {
        $p0ok = $true
        $overlap = $false
        foreach ($x in $ub0) { if ($ua0 -contains $x) { $overlap = $true } }
        $p1ok = (-not $overlap)
        $sd = Invoke-Api "U1" GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
        $d10 = "p0=$($ua0.Count) p1=$($ub0.Count) overlap=$overlap"
        if ($sd.Data) {
            $sUids = @($sd.Data | ForEach-Object { [uint32]$_.uid })
            $mono = $true
            for ($k = 1; $k -lt $sUids.Count; $k++) { if ($sUids[$k] -gt $sUids[$k-1]) { $mono = $false } }
            $sortOk = $mono
            $d10 += " descMonotone=$mono"
        }
        break
    }
}
Check P4.10 "pagination boundary (6 msgs: page0=6 page1=0); date desc monotonic" ($p0ok -and $p1ok -and $sortOk) "$d10 (multi-page deferred to P5; backend PageSize=50)"

# P4.11 attachments zip
$aTxt = [Text.Encoding]::UTF8.GetBytes("zip probe a $rnd")
$aB64 = [Convert]::ToBase64String($aTxt)
$m9 = "P4-zip-$rnd"
$zs = Invoke-Api "U1" POST "/mail/send" @{ to = @($U); subject = $m9; text = "zip"; attachments = @(@{ filename = "a1.txt"; content_type = "text/plain"; size = $aTxt.Length; data = $aB64 }, @{ filename = "a2.txt"; content_type = "text/plain"; size = $aTxt.Length; data = $aB64 }) }
$zu = Wait-Subject "U1" $m9 150
$zipOk = $false; $zipD = "send=$($zs.Status) uid=$zu"
if ($zu -gt 0) {
    try {
        $zr = $null
        try { $zr = Invoke-WebRequest -Uri "$Base/mail/attachments/zip?folder=Inbox&uid=$zu" -WebSession (Get-S "U1") -UseBasicParsing -TimeoutSec 60 } catch { $script:Auth401++; $Sessions["U1"].S = $null; $zr = Invoke-WebRequest -Uri "$Base/mail/attachments/zip?folder=Inbox&uid=$zu" -WebSession (Get-S "U1") -UseBasicParsing -TimeoutSec 60 }
        $bytes = $zr.Content
        $pk = ($bytes[0] -eq 0x50 -and $bytes[1] -eq 0x4B)
        $zipOk = ($zr.StatusCode -eq 200 -and $pk)
        $zipD += " status=$($zr.StatusCode) bytes=$($bytes.Length) PK=$pk"
    } catch {
        $code = 0; if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        $zipD += " error=$code"
    }
}
Check P4.11 "attachments zip returns PK magic" $zipOk $zipD

# P4.12 concurrency: session2 deletes m3 while session1 reads
Bind-Session "U2" $U "QaUser2026!"
$S2 = Get-S "U2"
$cu3 = Find-Uid "U2" "Inbox" $m3
$saw5xx = $false; $delOk = $false
if ($cu3 -gt 0) {
    $del2 = Invoke-Api "U2" POST "/mail/delete" @{ folder = "Inbox"; uid = $cu3 }
    for ($k = 0; $k -lt 5; $k++) {
        $rd = Invoke-Api "U1" GET "/mail/messages?folder=Inbox&page=0"
        if ($rd.Status -ge 500) { $saw5xx = $true }
        Start-Sleep -Milliseconds 300
    }
    $delOk = ($del2.Status -eq 204)
} else { $saw5xx = $false; $delOk = $false }
$fin = Invoke-Api "U1" GET "/mail/messages?folder=Inbox&page=0"
$m3left = 0
if ($fin.Data) { foreach ($m in @($fin.Data)) { if ($m.subject -eq $m3) { $m3left = 1 } } }
Check P4.12 "concurrent read+delete consistent, no 5xx" ($delOk -and (-not $saw5xx) -and $m3left -eq 0) "del=$delOk saw5xx=$saw5xx m3stillThere=$m3left"

"== auth401 self-heals: $script:Auth401 =="
# cleanup
$fdel = Invoke-Api "U1" DELETE "/mail/folders?name=$foldR"
$udel = Api $admin DELETE "/users/$U"
"cleanup: folder=$($fdel.Status) user=$($udel.Status)"
Flush-Results "P4"
"== done =="



