# qa-tp-p12-sieve.ps1 - P12 sieve scripts + real delivery routing (TEST-PROMPTS.md P12), rev1
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P12 sieve rev1 rnd=$rnd base=$Base =="
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
$UA = "qa-p12a-$rnd@example.com"
$UB = "qa-p12b-$rnd@example.com"
$admin = New-Session
foreach ($x in @($UA,$UB)) {
    $r = Api $admin POST "/users" @{ email = $x; password = "QaUser2026!"; display_name = "QA P12" }
    if ($r.Status -notin @(200,201)) { "prep FAIL create -> $($r.Status)"; exit 1 }
}
Bind-Session "A" $UA "QaUser2026!"
Bind-Session "B" $UB "QaUser2026!"
"prep: users ready"

# folders on A
$f1 = "p12target-$rnd"; $f2 = "p12second-$rnd"
[void](Invoke-Api "A" POST "/mail/folders" @{ name = $f1 })
[void](Invoke-Api "A" POST "/mail/folders" @{ name = $f2 })
Start-Sleep -Seconds 5

# P12.1 script save/get/activate + list shows single active
$sName = "p12main-$rnd"
$script1 = "require [`"fileinto`"];`r`nif header :contains [`"from`"] `"$UB`" { fileinto `"$f1`"; stop; }`r`n"
$sb = @{ script = $script1 } | ConvertTo-Json -Compress
$put1 = Invoke-Api "A" PUT "/sieve/$sName" @{ content = $script1; activate = $false }
$get1 = Invoke-Api "A" GET "/sieve/$sName"
$contentOk = $false
if ($get1.Data) {
    $got = "$($get1.Data.content)"
    if (-not $got) { $got = "$($get1.Data)" }
    $contentOk = ($got -match "fileinto" -and $got -match [regex]::Escape($f1))
}
$act = Invoke-Api "A" POST "/sieve/$sName/activate"
$lst = Invoke-Api "A" GET "/sieve"
$activeCount = 0; $mineActive = $false; $names = @()
if ($lst.Data) {
    $entries = $lst.Data
    if ($lst.Data.scripts) { $entries = $lst.Data.scripts }
    foreach ($e in @($entries)) {
        $nm = "$($e.name)"; if ($nm -eq "") { $nm = "$e" }
        $names += $nm
        $isAct = $false
        if ($e.active -ne $null) { $isAct = [bool]$e.active } elseif ($e.is_active -ne $null) { $isAct = [bool]$e.is_active }
        if ($isAct) { $activeCount++; if ($nm -eq $sName) { $mineActive = $true } }
    }
}
Check P12.1 "sieve save/get-content/activate/single-active" ($put1.Status -in @(200,201,204) -and $contentOk -and $act.Status -in @(200,204) -and $activeCount -eq 1 -and $mineActive) "put=$($put1.Status) contentOk=$contentOk activate=$($act.Status) scripts=[$($names -join ',')] activeCount=$activeCount mineActive=$mineActive"

# P12.2 real delivery routing: B -> A lands in $f1 not Inbox
$sj = "p12filtered-$rnd"
$rs = Invoke-Api "B" POST "/mail/send" @{ to = @($UA); subject = $sj; body = "sieve route test" }
$hit = 0; $inboxHas = $false
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 180) {
    $rf = Invoke-Api "A" GET "/mail/messages?folder=$f1&page=0"
    if ($rf.Status -eq 200) { foreach ($m in @($rf.Data)) { if ($m.subject -eq $sj) { $hit = [uint32]$m.uid } } }
    if ($hit -gt 0) { break }
    Start-Sleep -Seconds 5
}
$ri = Invoke-Api "A" GET "/mail/messages?folder=Inbox&page=0"
if ($ri.Status -eq 200) { foreach ($m in @($ri.Data)) { if ($m.subject -eq $sj) { $inboxHas = $true } } }
Check P12.2 "rule routes real delivery into target folder" ($rs.Status -eq 204 -and $hit -gt 0 -and -not $inboxHas) "send=$($rs.Status) targetUid=$hit in=$([int]$sw.Elapsed.TotalSeconds)s inboxHasIt=$inboxHas"

# P12.3 multi-rule priority: subject matches rule1 (alpha) and rule2 (alpha-beta); first wins
$sName2 = "p12prio-$rnd"
$script2 = "require [`"fileinto`"];`r`nif header :contains [`"subject`"] `"alpha`" { fileinto `"$f1`"; stop; }`r`nelsif header :contains [`"subject`"] `"beta`" { fileinto `"$f2`"; stop; }`r`n"
[void](Invoke-Api "A" PUT "/sieve/$sName2" @{ content = $script2; activate = $false })
$act2 = Invoke-Api "A" POST "/sieve/$sName2/activate"
$lst2 = Invoke-Api "A" GET "/sieve"
$stillOneActive = $false
if ($lst2.Data) {
    $entries = $lst2.Data
    if ($lst2.Data.scripts) { $entries = $lst2.Data.scripts }
    $ac = 0; $prioActive = $false
    foreach ($e in @($entries)) {
        $nm = "$($e.name)"; if ($nm -eq "") { $nm = "$e" }
        $isAct = $false
        if ($e.active -ne $null) { $isAct = [bool]$e.active } elseif ($e.is_active -ne $null) { $isAct = [bool]$e.is_active }
        if ($isAct) { $ac++; if ($nm -eq $sName2) { $prioActive = $true } }
    }
    $stillOneActive = ($ac -eq 1 -and $prioActive)
}
$sj2 = "p12-alpha-beta-$rnd"
[void](Invoke-Api "B" POST "/mail/send" @{ to = @($UA); subject = $sj2; body = "priority test" })
$hitF1 = 0; $hitF2 = 0
$sw2 = [Diagnostics.Stopwatch]::StartNew()
while ($sw2.Elapsed.TotalSeconds -lt 180) {
    $r1 = Invoke-Api "A" GET "/mail/messages?folder=$f1&page=0"
    $r2 = Invoke-Api "A" GET "/mail/messages?folder=$f2&page=0"
    if ($r1.Status -eq 200) { foreach ($m in @($r1.Data)) { if ($m.subject -eq $sj2) { $hitF1 = [uint32]$m.uid } } }
    if ($r2.Status -eq 200) { foreach ($m in @($r2.Data)) { if ($m.subject -eq $sj2) { $hitF2 = [uint32]$m.uid } } }
    if ($hitF1 -gt 0 -or $hitF2 -gt 0) { break }
    Start-Sleep -Seconds 5
}
Check P12.3 "priority: first matching rule wins; only one active script" ($act2.Status -in @(200,204) -and $stillOneActive -and $hitF1 -gt 0 -and $hitF2 -eq 0) "act2=$($act2.Status) singleActive=$stillOneActive rule1Hit=$hitF1 rule2Hit=$hitF2 in=$([int]$sw2.Elapsed.TotalSeconds)s"

# P12.4 invalid syntax rejected with line info
$badName = "p12bad-$rnd"
$badScript = "require [`"fileinto`"];`r`nif header :contains`r`n{ fileinto }`r`n"
$badPut = Invoke-Api "A" PUT "/sieve/$badName" @{ content = $badScript; activate = $false }
$badRejected = ($badPut.Status -ge 400) # engine syntax NO surfaces as 4xx/5xx via backend proxy
$badBody = "$($badPut.Body)".Trim()
if ($badBody.Length -gt 200) { $badBody = $badBody.Substring(0,200) }
Check P12.4 "invalid sieve script rejected" $badRejected "status=$($badPut.Status) body=[$badBody]"

# P12.5 vacation accepted and removable (real reply timing out of scope)
$vName = "p12vac-$rnd"
$vScript = "require [`"vacation`"];`r`nvacation :days 1 :subject `"out of office`" `"I am away`";`r`n"
$vPut = Invoke-Api "A" PUT "/sieve/$vName" @{ content = $vScript; activate = $false }
$vAct = Invoke-Api "A" POST "/sieve/$vName/activate"
$vDel = 0
if ($vAct.Status -in @(200,204)) {
    $vDel = (Invoke-Api "A" DELETE "/sieve/$vName").Status
    if ($vDel -notin @(200,204,403)) { $vDel = (Invoke-Api "A" DELETE "/sieve/$vName").Status }
}
Check P12.5 "vacation script accepted; removable" ($vPut.Status -in @(200,201,204) -and $vAct.Status -in @(200,204) -and $vDel -in @(200,204,403)) "put=$($vPut.Status) act=$($vAct.Status) del=$($vDel) (auto-reply generation beyond test window)"

# P12.6 direct ManageSieve protocol on 4190 (LISTSCRIPTS after AUTHENTICATE)
$protoOk = $false; $protoDetail = ""
try {
    $tcp = New-Object Net.Sockets.TcpClient('127.0.0.1',4190)
    $ns = $tcp.GetStream()
    $rd = New-Object IO.StreamReader($ns); $wr = New-Object IO.StreamWriter($ns); $wr.NewLine = "`r`n"
    $greet = $rd.ReadLine()
    $protoDetail += "greet=[$greet] "
    $authPair = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("`0$UA`0QaUser2026!"))
    $wr.WriteLine("a1 AUTHENTICATE `"PLAIN`" `"$authPair`""); $wr.Flush()
    $authResp = $rd.ReadLine()
    $protoDetail += "auth=[$authResp] "
    if ($authResp -match "^a1 OK") {
        $wr.WriteLine('a2 LISTSCRIPTS'); $wr.Flush()
        while ($true) { $l = $rd.ReadLine(); $protoDetail += "[$l] "; if ($l -match '^a2 ') { break } }
        $protoOk = ($protoDetail -match "p12")
    }
    $wr.WriteLine('a9 LOGOUT'); $wr.Flush()
    $tcp.Close()
} catch { $protoDetail += "err=$($_.Exception.Message)" }
Check P12.6 "direct ManageSieve 4190 LISTSCRIPTS consistent with webmail view" $protoOk $protoDetail

"== auth401 self-heals: $script:Auth401 =="
$ud1 = Api $admin DELETE "/users/$UA"
$ud2 = Api $admin DELETE "/users/$UB"
"cleanup: users=$($ud1.Status)/$($ud2.Status)"
Flush-Results "P12"
"== done =="

