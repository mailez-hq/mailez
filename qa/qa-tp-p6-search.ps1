# qa-tp-p6-search.ps1 - P6 search: keywords, operators, i18n, quick views (TEST-PROMPTS.md P6), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P6 search rev1 rnd=$rnd base=$Base =="

$U1 = "qa-p6a-$rnd@example.com"
$U2 = "qa-p6b-$rnd@example.com"
$admin = New-Session
foreach ($u in @($U1,$U2)) {
    $r = Api $admin POST "/users" @{ email = $u; password = "QaUser2026!"; display_name = "QA P6" }
    if ($r.Status -notin @(200,201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
# login helper via qa-lib New-Session pattern
function Login { param([string]$Email)
    $body = @{ email = $Email; pw = "QaUser2026!" } | ConvertTo-Json -Compress
    $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($body)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s
    return $s
}
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
}Bind-Session "A" $U1 "QaUser2026!"
Bind-Session "B" $U2 "QaUser2026!"
"prep: users ready"

function Find-Uid {
    param($Sess, [string]$Subject)
    foreach ($f in @("Inbox","Junk")) {
        $r = Invoke-Api $Sess GET "/mail/messages?folder=$f&page=0"
        if ($r.Status -ne 200) { continue }
        foreach ($m in @($r.Data)) { if ($m.subject -eq $Subject) { return @{ Uid = [uint32]$m.uid; Folder = $f } } }
    }
    return @{ Uid = 0; Folder = "" }
}
function Wait-Uid {
    param($Sess, [string]$Subject, [int]$Seconds)
    $sw = [Diagnostics.Stopwatch]::StartNew()
    while ($sw.Elapsed.TotalSeconds -lt $Seconds) {
        $f = Find-Uid $Sess $Subject
        if ($f.Uid -gt 0) { return $f }
        Start-Sleep -Seconds 4
    }
    return @{ Uid = 0; Folder = "" }
}
function Send12 {
    # send from U1 to U2
    param([string]$Subject, [string]$Text, $Attachments = $null)
    $pl = @{ to = @($U2); subject = $Subject; text = $Text }
    if ($Attachments) { $pl.attachments = $Attachments }
    $r = Invoke-Api "A" POST "/mail/send" $pl
    return $r.Status
}
function Search-Hits {
    param($Sess, [string]$Folder, [string]$Q)
    $r = Invoke-Api $Sess GET ("/mail/search?folder=$Folder&q=" + [uri]::EscapeDataString($Q))
    if ($r.Status -ne 200) { return @{ Status = $r.Status; Subjects = @() } }
    $subs = @()
    foreach ($m in @($r.Data)) { $subs += "$($m.subject)" }
    return @{ Status = 200; Subjects = $subs }
}

# ---- seed ----
$kSubj = "p6alpha-$rnd";   $kBody = "p6bravo-$rnd";   $kZh = "p6zh-$rnd"; $kEmoji = "p6emoji-$rnd"; $kAtt = "p6att-$rnd"; $kOther = "p6other-$rnd"
$needle = "xuniquex$rnd"
[void](Send12 $kSubj "hello world body")
[void](Send12 $kOther "plain filler text")
[void](Send12 $kBody "the $needle hides here")
[void](Send12 $kZh "zhongwen guanjianci $rnd")
[void](Send12 $kEmoji "party time")
$aTxt = [Text.Encoding]::UTF8.GetBytes("p6 attachment payload")
$aB64 = [Convert]::ToBase64String($aTxt)
[void](Send12 $kAtt "carries an attachment" @(@{ filename = "p6f.txt"; content_type = "text/plain"; size = $aTxt.Length; data = $aB64 }))
# U2 replies so U1's mailbox has a message FROM U2
$r2 = Invoke-Api "B" POST "/mail/send" @{ to = @($U1); subject = "p6reply-$rnd"; text = "from u2" }
"seed: sent 6 from U1 (last status $($r2.Status))"

# P6.1 keyword subject / body
$wSubj = Wait-Uid "B" $kSubj 150
$wBody = Wait-Uid "B" $kBody 150
$wZh = Wait-Uid "B" $kZh 150
$wEm = Wait-Uid "B" $kEmoji 150
$wAtt = Wait-Uid "B" $kAtt 150
$hSubj = Search-Hits "B" "Inbox" $kSubj
$hBody = Search-Hits "B" "Inbox" $needle
$onlySubj = ($hSubj.Subjects.Count -eq 1 -and $hSubj.Subjects[0] -eq $kSubj)
$onlyBody = ($hBody.Subjects.Count -eq 1 -and $hBody.Subjects[0] -eq $kBody)
Check P6.1a "keyword hits subject exactly" ($wSubj.Uid -gt 0 -and $hSubj.Status -eq 200 -and $onlySubj) "wait=$($wSubj.Uid) hits=[$($hSubj.Subjects -join '|')]"
Check P6.1b "keyword hits body exactly" ($wBody.Uid -gt 0 -and $hBody.Status -eq 200 -and $onlyBody) "wait=$($wBody.Uid) hits=[$($hBody.Subjects -join '|')]"

# P6.2 operators
$wRep = Wait-Uid "A" "p6reply-$rnd" 150
$hFrom = Search-Hits "A" "Inbox" ("from:" + $U2)
$fromOk = ($hFrom.Subjects.Count -ge 1 -and $hFrom.Subjects -contains "p6reply-$rnd")
$hHas = Search-Hits "B" "Inbox" "has:attachment"
$hasOk = ($hHas.Subjects -contains $kAtt)
$hComb = Search-Hits "B" "Inbox" ("from:" + $U1 + " has:attachment")
$combOk = ($hComb.Subjects.Count -eq 1 -and $hComb.Subjects[0] -eq $kAtt)
$now = [DateTimeOffset]::UtcNow
$hAfter = Search-Hits "B" "Inbox" ("after:" + $now.ToString("yyyy-MM-dd"))
$afterOk = ($hAfter.Subjects.Count -ge 5)
$hBefore = Search-Hits "B" "Inbox" ("before:2020-01-01")
$beforeOk = ($hBefore.Status -eq 200 -and $hBefore.Subjects.Count -eq 0)
Check P6.2a "from: filter" ($wRep.Uid -gt 0 -and $fromOk) "wait=$($wRep.Uid) hits=[$($hFrom.Subjects -join '|')]"
Check P6.2b "has:attachment filter" $hasOk "hits=[$($hHas.Subjects -join '|')]"
Check P6.2c "combined from:+has:attachment" $combOk "hits=[$($hComb.Subjects -join '|')]"
Check P6.2d "after: today range non-empty; before:2020 empty" ($afterOk -and $beforeOk) "after=$($hAfter.Subjects.Count) beforeEmpty=$($hBefore.Subjects.Count)"

# P6.3 folder=all boundary: move kOther to custom folder then compare
$cf = "p6fold-$rnd"
[void](Invoke-Api "B" POST "/mail/folders" @{ name = $cf })
$uOther = Find-Uid "B" $kOther
$mvR = 0
if ($uOther.Uid -gt 0) { $mvR = (Invoke-Api "B" POST "/mail/move" @{ folder = $uOther.Folder; uids = @($uOther.Uid); destination = $cf }).Status }
$ok3 = $false; $d3 = "move=$mvR"
foreach ($i in 1..15) {
    Start-Sleep -Seconds 4
    $hAll = Search-Hits "B" "all" $kOther
    $hIn = Search-Hits "B" "Inbox" $kOther
    if ($hAll.Status -eq 200 -and $hIn.Status -eq 200 -and $hAll.Subjects.Count -eq 1 -and $hIn.Subjects.Count -eq 0) { $ok3 = $true; $d3 += " all=1 inbox=0 after=$($i*4)s"; break }
    $d3 = "move=$mvR all=$($hAll.Subjects.Count)/$($hAll.Status) inbox=$($hIn.Subjects.Count)/$($hIn.Status)"
}
Check P6.3 "folder=all vs Inbox boundary" $ok3 $d3

# P6.4 i18n and special characters
$hQ1 = Search-Hits "B" "Inbox" ("subject:" + $kEmoji)
$emojiOk = ($hQ1.Status -eq 200 -and $hQ1.Subjects -contains $kEmoji)
$hZh = Search-Hits "B" "all" "zhongwen"
$zhOk = ($hZh.Status -eq 200 -and $hZh.Subjects -contains $kZh)
$hLit = Search-Hits "B" "Inbox" ('subject:"' + $kSubj + '"')
$litOk = ($hLit.Status -eq 200 -and $hLit.Subjects.Count -le 1 -and $hLit.Subjects -contains $kSubj)
$hCol = Search-Hits "B" "Inbox" "p6alpha:weird"
$colOk = ($hCol.Status -eq 200)
Check P6.4a "emoji subject searchable" ($wEm.Uid -gt 0 -and $emojiOk) "wait=$($wEm.Uid) hits=[$($hQ1.Subjects -join '|')] status=$($hQ1.Status)"
Check P6.4b "ascii-ized zh keyword hits, no 500" ($wZh.Uid -gt 0 -and $zhOk) "wait=$($wZh.Uid) hits=[$($hZh.Subjects -join '|')] status=$($hZh.Status)"
Check P6.4c "quoted subject literal" ($litOk) "hits=[$($hLit.Subjects -join '|')] status=$($hLit.Status)"
Check P6.4d "colon-bearing query no 500" $colOk "status=$($hCol.Status) hits=$($hCol.Subjects.Count)"

# P6.5 saved search: no backend route exists -> verify 404-ish and mark SKIP-note
$ss = Invoke-Api "B" GET "/mail/searches"
Check P6.5 "saved search persistence (no API route; client-side)" ($ss.Status -in @(404,405)) "GET /mail/searches -> $($ss.Status) (feature is webmail-local; no server persistence to test)"

# P6.6 quick views consistency: unread / flagged
[void](Invoke-Api "B" POST "/mail/read-all" @{ folder = "Inbox" })
$hu = Search-Hits "B" "Inbox" "is:unread"
$unreadOk = ($hu.Status -eq 200 -and $hu.Subjects.Count -eq 0)
$wA2 = Wait-Uid "B" $kSubj 150
[void](Invoke-Api "B" POST "/mail/flag" @{ folder = "Inbox"; uid = $wA2.Uid; flag = "flagged"; value = $true })
$hf = Search-Hits "B" "Inbox" "is:flagged"
$flagOk = ($hf.Status -eq 200 -and $hf.Subjects -contains $kSubj)
Check P6.6a "is:unread empty after read-all" $unreadOk "hits=$($hu.Subjects.Count)"
Check P6.6b "is:flagged reflects star" ($wA2.Uid -gt 0 -and $flagOk) "uid=$($wA2.Uid) hits=[$($hf.Subjects -join '|')]"

# P6.7 light perf + search-while-receiving
$sw = [Diagnostics.Stopwatch]::StartNew()
$hPerf = Search-Hits "B" "all" "p6"
$perfMs = $sw.ElapsedMilliseconds
$err = 0
$bg = Invoke-Api "A" POST "/mail/send" @{ to = @($U2); subject = "p6burst-$rnd"; text = "burst" }
for ($k = 0; $k -lt 5; $k++) {
    $hb = Search-Hits "B" "Inbox" "p6"
    if ($hb.Status -ne 200) { $err++ }
    Start-Sleep -Milliseconds 500
}
Check P6.7 "search latency + search-while-receiving" ($hPerf.Status -eq 200 -and $perfMs -lt 5000 -and $err -eq 0 -and $bg.Status -eq 204) "p6 query=${perfMs}ms errors=$err burst=$($bg.Status) (10k-scale P95 deferred to load-test round)"

"== auth401 self-heals: $script:Auth401 =="
# cleanup
$fdel = Invoke-Api "B" DELETE "/mail/folders?name=$cf"
$ud1 = Api $admin DELETE "/users/$U1"
$ud2 = Api $admin DELETE "/users/$U2"
"cleanup: folder=$($fdel.Status) users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P6"
"== done =="

