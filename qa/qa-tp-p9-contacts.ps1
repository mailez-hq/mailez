# qa-tp-p9-contacts.ps1 - P9 contacts + vCard + dedupe + CardDAV config (TEST-PROMPTS.md P9), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P9 contacts rev1 rnd=$rnd base=$Base =="
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
$U = "qa-p9a-$rnd@example.com"
$P = "qa-p9b-$rnd@example.com"
$admin = New-Session
foreach ($x in @($U,$P)) {
    $r = Api $admin POST "/users" @{ email = $x; password = "QaUser2026!"; display_name = "QA P9" }
    if ($r.Status -notin @(200,201)) { "prep FAIL create -> $($r.Status)"; exit 1 }
}
Bind-Session "A" $U "QaUser2026!"
Bind-Session "B" $P "QaUser2026!"
"prep: users ready"

# P9.1 CRUD
$c1 = Invoke-Api "A" POST "/contacts" @{ name = "Alice Zhang"; email = "alice-$rnd@example.com"; comment = "tel:+8613800000000 org:Acme"; groups = "friends" }
$cid = 0; if ($c1.Data -and $c1.Data.ID) { $cid = $c1.Data.ID } elseif ($c1.Data -and $c1.Data.id) { $cid = $c1.Data.id }
$ls = Invoke-Api "A" GET "/contacts"
$found = $false; $cnt = 0
if ($ls.Data) { foreach ($x in @($ls.Data)) { $cnt++; if ("$($x.email)" -eq "alice-$rnd@example.com") { $found = $true } } }
$up = Invoke-Api "A" PUT "/contacts/$cid" @{ name = "Alice Zhang-Wang"; comment = "tel:+8613900000000 org:Acme Corp" }
$ls2 = Invoke-Api "A" GET "/contacts"
$nameUp = $false
if ($ls2.Data) { foreach ($x in @($ls2.Data)) { if ("$($x.email)" -eq "alice-$rnd@example.com" -and "$($x.name)" -eq "Alice Zhang-Wang") { $nameUp = $true } } }
Check P9.1 "contact create/list/update" ($c1.Status -in @(200,201) -and $cid -gt 0 -and $found -and $up.Status -in @(200,204) -and $nameUp) "create=$($c1.Status) id=$cid listed=$found(count=$cnt) update=$($up.Status) nameUpdated=$nameUp"

# P9.2 vCard export roundtrip
$c2 = Invoke-Api "A" POST "/contacts" @{ name = "Bob Li"; email = "bob-$rnd@example.com"; comment = "tel:+8613911111111 org:Globex"; groups = "work" }
$ex = Invoke-Api "A" GET "/contacts/export"
$vcf = "$($ex.Content)"
$hasBegin = ($vcf -match "BEGIN:VCARD")
$hasAlice = ($vcf -match "alice-$rnd@example.com")
$hasBob = ($vcf -match "bob-$rnd@example.com")
$hasTel = ($vcf -match "\+86139")
$hasOrg = ($vcf -match "Acme")
$ctypeOk = ("$($ex.Headers['Content-Type'])" -match "vcard") -or ("$($ex.Headers['Content-Type'])" -match "text/vcard")
Check P9.2 "vCard export contains entries and fields" ($c2.Status -in @(200,201) -and $ex.Status -eq 200 -and $hasBegin -and $hasAlice -and $hasBob -and $hasOrg -and $ctypeOk) "begin=$hasBegin alice=$hasAlice bob=$hasBob tel=$hasTel org=$hasOrg ctype=$($ex.Headers['Content-Type']) (TEL folding: tel strings live in comment: $hasTel)"

# P9.3 import with duplicates
$vcfImport = "BEGIN:VCARD`r`nVERSION:3.0`r`nFN:Carol Chen`r`nN:Chen;Carol`r`nEMAIL:carol-$rnd@example.com`r`nTEL:+8613922222222`r`nORG:Initech`r`nEND:VCARD`r`nBEGIN:VCARD`r`nVERSION:3.0`r`nFN:Dave Wu`r`nN:Wu;Dave`r`nEMAIL:dave-$rnd@example.com`r`nEND:VCARD`r`nBEGIN:VCARD`r`nVERSION:3.0`r`nFN:Alice Duplicate`r`nN:Duplicate;Alice`r`nEMAIL:alice-$rnd@example.com`r`nEND:VCARD`r`n"
$im = Invoke-Api "A" POST "/contacts/import" @{ data = $vcfImport }
$added = -1; $total = -1
if ($im.Data) { $added = [int]$im.Data.added; $total = [int]$im.Data.total }
$im2 = Invoke-Api "A" POST "/contacts/import" @{ data = $vcfImport }
$added2 = -1; if ($im2.Data) { $added2 = [int]$im2.Data.added }
Check P9.3 "vCard import adds new, skips duplicate" ($im.Status -eq 200 -and $added -eq 2 -and $total -eq 3 -and $added2 -eq 0) "first: added=$added/$total reimport added=$added2"

# P9.4 dedupe merge
$dup1 = Invoke-Api "A" POST "/contacts" @{ name = "Frank Short"; email = "frank-$rnd@example.com"; comment = "short" }
$dup2 = Invoke-Api "A" POST "/contacts" @{ name = "Frank Longer Name Here"; email = "FRANK-$rnd@example.com"; comment = "longer comment payload"; groups = "vend" }
$dd = Invoke-Api "A" POST "/contacts/dedupe"
$ls3 = Invoke-Api "A" GET "/contacts"
$frankCount = 0; $frankName = ""; $frankComment = ""; $frankGroups = ""
if ($ls3.Data) { foreach ($x in @($ls3.Data)) { if ("$($x.email)" -like "frank-*@$($x.email.Split('@')[1])") {} ; if ("$($x.email)".ToLower() -eq "frank-$rnd@example.com") { $frankCount++; $frankName = "$($x.name)"; $frankComment = "$($x.comment)"; $frankGroups = "$($x.groups)" } } }
$ddMerged = -1; if ($dd.Data) { $ddMerged = [int]$dd.Data.merged }
Check P9.4 "dedupe merges same-email entries, keeps most complete" ($dup1.Status -in @(200,201) -and $dup2.Status -in @(200,201) -and $dd.Status -eq 200 -and $ddMerged -ge 1 -and $frankCount -eq 1 -and $frankName -eq "Frank Longer Name Here" -and $frankComment -match "longer comment") "merged=$ddMerged frankEntries=$frankCount name=$frankName comment=[$frankComment] groups=[$frankGroups]"

# P9.5 autocomplete data source (list is client-filtered)
$ac = Invoke-Api "A" GET "/contacts"
$acHit = $false
if ($ac.Data) { foreach ($x in @($ac.Data)) { if ("$($x.email)" -like "carol-$rnd*") { $acHit = $true } } }
Check P9.5 "autocomplete source list contains imported contact" ($ac.Status -eq 200 -and $acHit) "server list has carol entry=$acHit (webmail filters client-side; compose autocomplete UI check deferred to P21)"

# P9.6 CardDAV config semantics + sync error handling
$gd0 = Invoke-Api "A" GET "/contacts/carddav"
$st = Invoke-Api "A" PUT "/contacts/carddav" @{ url = "https://dav.example.invalid/alice"; username = "alice"; password = "davpass-$rnd" }
$gd1 = Invoke-Api "A" GET "/contacts/carddav"
$gd1txt = "$($gd1.Content)"
$pwLeak = ($gd1txt -match "davpass-$rnd")
$sy = Invoke-Api "A" POST "/contacts/carddav/sync"
$syOk = ($sy.Status -lt 500)
# restore empty config
[void](Invoke-Api "A" PUT "/contacts/carddav" @{ url = ""; username = ""; password = "" })
Check P9.6 "carddav config set/get; sync fails clean; no plaintext password" ($gd0.Status -in @(200,404) -and $st.Status -in @(200,204) -and $gd1.Status -eq 200 -and (-not $pwLeak) -and $syOk) "get0=$($gd0.Status) set=$($st.Status) get1=$($gd1.Status) pwLeaked=$pwLeak sync=$($sy.Status) body=$("$($sy.Body)".Trim().Substring(0,[Math]::Min(120,"$($sy.Body)".Length)))"

# P9.7 cross-user isolation on contacts (quick IDOR check, formal in P20)
$iso = Invoke-Api "B" PUT "/contacts/$cid" @{ name = "hijack" }
$isoOk = ($iso.Status -in @(403,404))
Check P9.7 "user B cannot modify A's contact" $isoOk "status=$($iso.Status)"

$lsF = Invoke-Api "A" GET "/contacts"
$delN = 0
if ($lsF.Data) { foreach ($x in @($lsF.Data)) { $did = $x.id; if (-not $did) { $did = $x.ID }; $dd2 = Invoke-Api "A" DELETE "/contacts/$did"; if ($dd2.Status -eq 204) { $delN++ } } }
"== auth401 self-heals: $script:Auth401 =="
$ud1 = Api $admin DELETE "/users/$U"
$ud2 = Api $admin DELETE "/users/$P"
"cleanup: contactsDeleted=$delN users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P9"
"== done =="
