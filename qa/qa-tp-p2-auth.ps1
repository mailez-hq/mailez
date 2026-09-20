# qa-tp-p2-auth.ps1 - P2 auth & session security (qa/TEST-PROMPTS.md P2)
# Env QA_BASE selects target API (default dev 8080). All test data uses qa- prefix.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P2 auth/session rnd=$rnd base=$Base =="

function Invoke-Login {
    param([string]$Email, [string]$Pw)
    $body = @{ email = $Email; pw = $Pw } | ConvertTo-Json -Compress
    try {
        $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($body)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s
        return @{ Status = [int]$r.StatusCode; S = $s; Body = $r.Content; SetCookie = "$($r.Headers['Set-Cookie'])" }
    } catch {
        $code = 0; $bd = ""
        if ($_.Exception.Response) {
            $code = [int]$_.Exception.Response.StatusCode
            try { $sr = New-Object IO.StreamReader($_.Exception.Response.GetResponseStream()); $bd = $sr.ReadToEnd() } catch {}
        }
        return @{ Status = $code; S = $null; Body = $bd; SetCookie = "" }
    }
}

# RFC 6238 TOTP (SHA1, 6 digits, 30s step), base32 secret
function Get-TotpCode {
    param([string]$Secret)
    $alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
    $clean = ($Secret.ToUpper()) -replace '[^A-Z2-7]',''
    $bits = ""
    foreach ($ch in $clean.ToCharArray()) { $bits += [Convert]::ToString($alpha.IndexOf($ch), 2).PadLeft(5, '0') }
    $n = [int][Math]::Floor($bits.Length / 8)
    $key = New-Object byte[] $n
    for ($i = 0; $i -lt $n; $i++) { $key[$i] = [Convert]::ToByte($bits.Substring($i * 8, 8), 2) }
    $counter = [UInt64](([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()) / 30)
    $ctr = [BitConverter]::GetBytes($counter); [Array]::Reverse($ctr)
    $hmac = New-Object Security.Cryptography.HMACSHA1 (,$key)
    $hash = $hmac.ComputeHash($ctr)
    $off = $hash[$hash.Length - 1] -band 0x0F
    $bin = ((($hash[$off] -band 0x7F) -shl 24) -bor (($hash[$off+1] -band 0xFF) -shl 16) -bor (($hash[$off+2] -band 0xFF) -shl 8) -bor ($hash[$off+3] -band 0xFF))
    return "{0:D6}" -f ($bin % 1000000)
}

$created = New-Object System.Collections.Generic.List[string]

# ---- prep: admin creates qa user ----
$admin = New-Session
$u1 = "qa-p2a-$rnd@example.com"
$nu = Api $admin POST "/users" @{ email = $u1; password = "QaUser2026!"; display_name = "QA P2 Auth" }
"prep: create $u1 -> $($nu.Status)"
$created.Add($u1)

# P2.1 correct login sets session cookie
$l1 = Invoke-Login -Email $u1 -Pw "QaUser2026!"
$cc = 0; if ($null -ne $l1.S) { $cc = $l1.S.Cookies.GetCookies($Base).Count }
Check P2.1 "correct login 200 + session cookie" ($l1.Status -eq 200 -and $cc -ge 1) "status=$($l1.Status) cookies=$cc"

# P2.2 wrong password 401, no session
$l2 = Invoke-Login -Email $u1 -Pw "Wrong-$rnd!"
Check P2.2 "wrong password 401 no session" ($l2.Status -eq 401 -and $null -eq $l2.S) "status=$($l2.Status) body=$($l2.Body)"

# P2.3 /sso/me returns login email
$me = Api $l1 GET "/sso/me"
Check P2.3 "/sso/me returns login email" ($me.Status -eq 200 -and $me.Data.email -eq $u1) "status=$($me.Status) email=$($me.Data.email)"

# P2.4 session cookie HttpOnly
$sc = $l1.SetCookie
$httponly = $sc -match 'HttpOnly'
Check P2.4 "session cookie HttpOnly" ($httponly) "set-cookie=$sc"

# P2.5 settings: change display name/signature, read back
$disp = "QADisp$rnd"
$p1 = Api $l1 PUT "/me/settings" @{ displayed_name = $disp; signature = "sig-$rnd" }
$g1 = Api $l1 GET "/me"; $g2 = Api $l1 GET "/sso/me"
Check P2.5 "settings change read-back" ($p1.Status -in @(200,204) -and $g1.Data.displayed_name -eq $disp -and $g2.Data.displayed_name -eq $disp) "put=$($p1.Status) me=$($g1.Data.displayed_name) sso=$($g2.Data.displayed_name)"

# P2.6 password cycle: change -> old fails -> new works -> revert
$c1 = Api $l1 PUT "/me/password" @{ old_pw = "QaUser2026!"; new_pw = "QaNew2026!" }
$o1 = Invoke-Login -Email $u1 -Pw "QaUser2026!"
$n1 = Invoke-Login -Email $u1 -Pw "QaNew2026!"
$rb = Api $n1 PUT "/me/password" @{ old_pw = "QaNew2026!"; new_pw = "QaUser2026!" }
$re = Invoke-Login -Email $u1 -Pw "QaUser2026!"
Check P2.6 "password change cycle" ($c1.Status -eq 204 -and $o1.Status -eq 401 -and $n1.Status -eq 200 -and $rb.Status -eq 204 -and $re.Status -eq 200) "chg=$($c1.Status) old=$($o1.Status) new=$($n1.Status) back=$($rb.Status) re=$($re.Status)"

# P2.7 logout then /sso/me 401
$l3 = Invoke-Login -Email $u1 -Pw "QaUser2026!"
$lo = Api $l3 POST "/sso/logout" @{}
$m3 = Api $l3 GET "/sso/me"
Check P2.7 "logout then 401" ($lo.Status -in @(200,204) -and $m3.Status -eq 401) "logout=$($lo.Status) me=$($m3.Status)"

# P2.8 login history
$lg = Api $re GET "/me/logins"
$lgn = 0; if ($lg.Data) { $lgn = @($lg.Data).Count }
Check P2.8 "/me/logins has local login" ($lg.Status -eq 200 -and $lgn -ge 1) "status=$($lg.Status) rows=$lgn"

# P2.9 app token create/list/delete
$t = Api $re POST "/tokens" @{ ip = "qa-tp-p2" }
$tid = $null; if ($t.Data) { $tid = $t.Data.id }
$tl = Api $re GET "/tokens?page=1&limit=50"
$hit = $false; if ($tl.Data -and $tl.Data.data) { $hit = @($tl.Data.data | Where-Object { "$($_.id)" -eq "$tid" }).Count -ge 1 }
$tdel = $null; if ($tid) { $tdel = Api $re DELETE "/tokens/$tid" }
$tl2 = Api $re GET "/tokens?page=1&limit=50"
$gone = $true; if ($tl2.Data -and $tl2.Data.data) { $gone = @($tl2.Data.data | Where-Object { "$($_.id)" -eq "$tid" }).Count -eq 0 }
Check P2.9 "app token create/list/delete" ($t.Status -eq 201 -and $hit -and $tdel.Status -eq 204 -and $gone) "create=$($t.Status) hit=$hit del=$($tdel.Status) gone=$gone"

# ---- P2.10 TOTP 2FA full flow ----
$totpSecret = $null
$ts = Api $re GET "/me/totp"
if ($ts.Status -eq 200 -and $ts.Data) {
    if ($ts.Data.secret) { $totpSecret = $ts.Data.secret }
    elseif ($ts.Data.otpauth -match 'secret=([A-Z2-7]+)') { $totpSecret = $Matches[1] }
}
$tEn = $null
if ($totpSecret) {
    $code1 = Get-TotpCode -Secret $totpSecret
    $tEn = Api $re POST "/me/totp/enable" @{ code = $code1 }
}
$st = Api $re GET "/me/totp"
$enabledNow = ($st.Data -and $st.Data.enabled)
Check P2.10a "TOTP enroll+enable" ($ts.Status -eq 200 -and $totpSecret -and $tEn.Status -in @(200,204) -and $enabledNow) "secret=$($null -ne $totpSecret) enable=$($tEn.Status) enabled=$enabledNow"

$pendingSeen = $false; $wrongRejected = $false; $totpLoginOk = $false; $totpMeOk = $false; $disStatus = 0; $plainAgain = $false
$s2obj = $null
if ($enabledNow) {
    # password step: 200 + totp_required + pending_token, no session cookie
    $pl = Invoke-Login -Email $u1 -Pw "QaUser2026!"
    $pendingSeen = ($pl.Status -eq 200 -and $pl.Body -match 'totp_required"?: ?true' -and $pl.Body -match 'pending_token')
    $pt = $null; if ($pl.Body -match '"pending_token":"([^"]+)"') { $pt = $Matches[1] }
    # wrong code rejected
    if ($pt) {
        $bd = @{ pending_token = $pt; code = "000000" } | ConvertTo-Json -Compress
        try {
            $wr = Invoke-WebRequest -Uri "$Base/sso/login/totp" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($bd)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 30
            $wrongRejected = ([int]$wr.StatusCode -eq 401)
        } catch { $wrongRejected = ($_.Exception.Response -and [int]$_.Exception.Response.StatusCode -eq 401) }
    }
    # correct code -> real session
    $pl2 = Invoke-Login -Email $u1 -Pw "QaUser2026!"
    $pt2 = $null; if ($pl2.Body -match '"pending_token":"([^"]+)"') { $pt2 = $Matches[1] }
    if ($pt2) {
        $code2 = Get-TotpCode -Secret $totpSecret
        $bd2 = @{ pending_token = $pt2; code = $code2 } | ConvertTo-Json -Compress
        try {
            $ok = Invoke-WebRequest -Uri "$Base/sso/login/totp" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($bd2)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 30 -SessionVariable s2
            $totpLoginOk = ([int]$ok.StatusCode -eq 200)
            $s2obj = @{ S = $s2 }
            $m4 = Api $s2obj GET "/sso/me"
            $totpMeOk = ($m4.Status -eq 200 -and $m4.Data.email -eq $u1)
        } catch { $totpLoginOk = $false }
    }
    # disable TOTP (needs current code)
    if ($s2obj) {
        $code3 = Get-TotpCode -Secret $totpSecret
        $tdis = Api $s2obj DELETE "/me/totp" @{ code = $code3 }
        $disStatus = $tdis.Status
    }
    $pl3 = Invoke-Login -Email $u1 -Pw "QaUser2026!"
    $plainAgain = ($pl3.Status -eq 200 -and $pl3.Body -notmatch 'totp_required')
    Check P2.10b "TOTP login flow (pending/wrong-reject/correct/me)" ($pendingSeen -and $wrongRejected -and $totpLoginOk -and $totpMeOk) "pending=$pendingSeen wrongrej=$wrongRejected login=$totpLoginOk me=$totpMeOk"
    Check P2.10c "TOTP disable restores plain login" ($disStatus -in @(200,204) -and $plainAgain) "disable=$disStatus plainlogin=$plainAgain"
} else {
    Check P2.10a "TOTP enroll+enable" $false "status=$($ts.Status) secret=$($null -ne $totpSecret) enable=$($tEn.Status)"
}

# ---- P2.11 login rate limiting ----
$u2 = "qa-p2rl-$rnd@example.com"
$nu2 = Api $admin POST "/users" @{ email = $u2; password = "QaUser2026!"; display_name = "QA P2 RL" }
$created.Add($u2)
$locked = $false; $attempts = 0; $lockMsg = ""
for ($i = 1; $i -le 15; $i++) {
    $attempts = $i
    $rl = Invoke-Login -Email $u2 -Pw "TotallyWrong$i!"
    if ($rl.Status -eq 429) { $locked = $true; $lockMsg = $rl.Body; break }
}
$still = Invoke-Login -Email $u2 -Pw "QaUser2026!"
$stillLocked = ($still.Status -eq 429)
Check P2.11 "failed logins trigger 429 lockout; correct pw also blocked while locked" ($locked -and $attempts -le 15 -and $stillLocked) "attempts=$attempts msg=$lockMsg correctDuringLock=$($still.Status)"

# P2.12 WebAuthn - SKIP here (needs virtual authenticator; covered by P21 UI)
Check P2.12 "WebAuthn register/auth" $true "SKIP: needs browser virtual authenticator, deferred to P21"

"== created for cleanup: $($created -join ', ') =="
foreach ($u in $created) {
    $ul = Api $admin GET "/users?page=1&limit=200"
    $id = $null
    if ($ul.Data -and $ul.Data.data) { $hit2 = $ul.Data.data | Where-Object { $_.email -eq $u } | Select-Object -First 1; if ($hit2) { $id = $hit2.id } }
    if ($id) { $d = Api $admin DELETE "/users/$id"; "cleanup: DELETE $u -> $($d.Status)" } else { "cleanup: $u id not found, left for final cleanup" }
}
Flush-Results "P2"
"== done =="
