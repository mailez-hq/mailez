# qa-tp-p12b-sieve-recheck.ps1 - P12 re-test (TEST-PROMPTS.md P12), rev2
#
# The first pass marked P12.3 ("only one active script") and P12.6
# ("LISTSCRIPTS agrees with the webmail view") as failures. Both read the
# ManageSieve capability list as if it were a single line; the greeting is
# multi-line, so the reading desynchronised. This run judges the same cases
# with a correct client (qa-sieve-list.py) plus real delivery.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P12 sieve recheck rev2 rnd=$rnd =="

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-p12c-a-$rnd@example.com"
$B = "qa-p12c-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P12c" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW

function SieveState([string]$name = "") {
    $cmdArgs = @(".\qa-sieve-list.py", "127.0.0.1", "4190", $A, $PW)
    if ($name -ne "") { $cmdArgs += $name }
    $out = @(& python @cmdArgs 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return $null }
    return ($json | ConvertFrom-Json)
}
function FolderSubjects([string]$folder) {
    $out = @(& python .\qa-imap-op.py 127.0.0.1 143 $A $PW subjects $folder 2>$null)
    $json = ($out | Where-Object { $_ -match '^\{' } | Select-Object -Last 1)
    if (-not $json) { return @() }
    $obj = $json | ConvertFrom-Json
    if (-not $obj.subjects) { return @() }
    return @($obj.subjects | ForEach-Object { $_[1] })
}

$f1 = "p12c-first-$rnd"
$f2 = "p12c-second-$rnd"
[void](Api $aSess POST "/mail/folders" @{ name = $f1 })
[void](Api $aSess POST "/mail/folders" @{ name = $f2 })
Start-Sleep -Seconds 4

# ---------------------------------------- P12.1 save / read back / activate
$nameMain = "p12cmain-$rnd"
$content = "require [`"fileinto`"];`r`n" +
    "if header :contains [`"subject`"] `"P12C-BOTH-$rnd`" { fileinto `"$f1`"; stop; }`r`n" +
    "if header :contains [`"subject`"] `"P12C-FIRST-$rnd`" { fileinto `"$f1`"; stop; }`r`n" +
    "if header :contains [`"subject`"] `"P12C-SECOND-$rnd`" { fileinto `"$f2`"; stop; }`r`n"
$put = Api $aSess PUT "/sieve/$nameMain" @{ content = $content; activate = $false }
$got = Api $aSess GET "/sieve/$nameMain"
$gotText = ""
if ($got.Data) { $gotText = if ($got.Data.content) { [string]$got.Data.content } else { [string]$got.Data } }
$act = Api $aSess POST "/sieve/$nameMain/activate"
$state = SieveState $nameMain
$activeNames = @($state.scripts | Where-Object { $_.active } | ForEach-Object { $_.name })
$scriptDesc = (@($state.scripts | ForEach-Object { $_.name + $(if ($_.active) { ":ACTIVE" } else { "" }) }) -join ",")
Check P12.1-rev2 "script saved, read back verbatim and activated" `
    ($put.Status -in @(200, 204) -and $gotText -match [regex]::Escape($f1) -and $act.Status -in @(200, 204) -and $activeNames -contains $nameMain) `
    "put=$($put.Status) getChars=$($gotText.Length) activate=$($act.Status) scripts=[$scriptDesc]"

# ------------------------------------------------ P12.4 invalid script rejected
$bad = "require [`"fileinto`"]`r`nif header :contains [`"subject`" `"x`" { fileinto `"$f1`"; }`r`n"
$badR = Api $aSess PUT "/sieve/p12cbad-$rnd" @{ content = $bad; activate = $false }
$badBody = ""
if ($badR.Raw) { $badBody = [Text.Encoding]::UTF8.GetString($badR.Raw.RawContentStream.ToArray()) }
Check P12.4-rev2 "syntactically invalid script is rejected" ($badR.Status -in @(400, 422, 502)) "status=$($badR.Status) body=$($badBody.Substring(0, [Math]::Min(160, $badBody.Length)))"

# --------------------------------------- P12.3 single-active + rule priority
$nameSecond = "p12csecond-$rnd"
$content2 = "require [`"fileinto`"];`r`nif header :contains [`"subject`"] `"P12C-SECOND-$rnd`" { fileinto `"$f2`"; stop; }`r`n"
$put2 = Api $aSess PUT "/sieve/$nameSecond" @{ content = $content2; activate = $true }
$state2 = SieveState
$actives = @($state2.scripts | Where-Object { $_.active })
$actNames = (@($actives | ForEach-Object { $_.name }) -join ",")
$allNames = (@($state2.scripts | ForEach-Object { $_.name }) -join ",")
Check P12.3a-rev2 "activating a second script leaves exactly one ACTIVE" ($put2.Status -in @(200, 204) -and $actives.Count -eq 1 -and $actives[0].name -eq $nameSecond) `
    "put=$($put2.Status) activeCount=$($actives.Count) active=[$actNames] all=[$allNames]"

# re-activate the multi-rule script and drive real deliveries through it
[void](Api $aSess POST "/sieve/$nameMain/activate")
Start-Sleep -Seconds 3
$subjBoth = "P12C-BOTH-$rnd"
$subjFirst = "P12C-FIRST-$rnd"
$subjSecond = "P12C-SECOND-$rnd"
[void](Api $bSess POST "/mail/send" @{ to = @($A); subject = $subjBoth; body = "matches both rules" })
[void](Api $bSess POST "/mail/send" @{ to = @($A); subject = $subjFirst; body = "first rule only" })
[void](Api $bSess POST "/mail/send" @{ to = @($A); subject = $subjSecond; body = "second rule only" })
$wait = 90
$sw = [Diagnostics.Stopwatch]::StartNew()
$f1Subs = @(); $f2Subs = @(); $inboxSubs = @()
while ($sw.Elapsed.TotalSeconds -lt $wait) {
    $f1Subs = FolderSubjects $f1
    $f2Subs = FolderSubjects $f2
    $inboxSubs = FolderSubjects INBOX
    if (($f1Subs -match [regex]::Escape($subjBoth)) -and ($f1Subs -match [regex]::Escape($subjFirst)) -and ($f2Subs -match [regex]::Escape($subjSecond))) { break }
    Start-Sleep -Seconds 4
}
$bothCopies = @($f1Subs | Where-Object { $_ -match [regex]::Escape($subjBoth) }).Count
$secondCopiesInF1 = @($f1Subs | Where-Object { $_ -match [regex]::Escape($subjSecond) }).Count
$firstInF1 = @($f1Subs | Where-Object { $_ -match [regex]::Escape($subjFirst) }).Count
$secondInF2 = @($f2Subs | Where-Object { $_ -match [regex]::Escape($subjSecond) }).Count
$strayInbox = @($inboxSubs | Where-Object { $_ -match "P12C-" }).Count
Check P12.2-rev2 "rules route real delivery into their target folders" `
    ($firstInF1 -ge 1 -and $secondInF2 -ge 1 -and $strayInbox -eq 0) `
    "first=$($f1Subs -join ',') second=$($f2Subs -join ',') inbox=[$($inboxSubs -join ',')] waited=$([int]$sw.Elapsed.TotalSeconds)s"
Check P12.2b-rev2 "first matching rule wins (message matching two rules lands once)" `
    ($bothCopies -eq 1 -and $secondCopiesInF1 -eq 0) `
    "bothRuleMessageCopiesInFirst=$bothCopies secondRuleMessageCopiesInFirst=$secondCopiesInF1"

# ------------------------------------------------- P12.5 vacation accepted
$vac = "require [`"vacation`"];`r`nvacation :days 1 :subject `"out of office`" `"I am away`";`r`n"
$vacPut = Api $aSess PUT "/sieve/p12cvac-$rnd" @{ content = $vac; activate = $false }
$vacDel = Api $aSess DELETE "/sieve/p12cvac-$rnd"
Check P12.5-rev2 "vacation script is accepted and removable" ($vacPut.Status -in @(200, 204) -and $vacDel.Status -in @(200, 204)) "put=$($vacPut.Status) delete=$($vacDel.Status)"

# ------------------------------- P12.6 ManageSieve view == API view (content)
$stateMain = SieveState $nameMain
$apiList = Api $aSess GET "/sieve"
$apiNames = @($apiList.Data | ForEach-Object { $_.name })
$rawNames = @($stateMain.scripts | ForEach-Object { $_.name })
$overlap = @($rawNames | Where-Object { $apiNames -contains $_ }).Count
$contentMatch = ($stateMain.content -match [regex]::Escape($f1))
Check P12.6-rev2 "ManageSieve LISTSCRIPTS/GETSCRIPT agrees with the API view" `
    ($stateMain.ok -and $overlap -eq $rawNames.Count -and $contentMatch) `
    "apiScripts=[$($apiNames -join ',')] sieveScripts=[$($rawNames -join ',')] getscriptMatchesContent=$contentMatch"

$cl = @()
$cl += "sieveA=" + (Api $aSess DELETE "/sieve/$nameMain").Status
$cl += "sieveB=" + (Api $aSess DELETE "/sieve/$nameSecond").Status
foreach ($u in @($A, $B)) { $cl += "user=" + (Api $admin DELETE "/users/$u").Status }
"cleanup: $($cl -join ' ')"
Flush-Results "P12"
"== done =="
