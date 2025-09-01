# qa-batch-a.ps1 — A1..A8 认证与会话
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== A 认证与会话 rnd=$rnd =="
# 与 qa-lib 相同的 Base 解析逻辑（QA_BASE 环境变量优先），仅供本脚本裸调用登录用
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }

# 裸登录：不经过 Api（Api 需要 WebSession），返回状态码与会话
function Invoke-Login {
    param([string]$Email, [string]$Pw)
    $body = @{ email = $Email; pw = $Pw } | ConvertTo-Json -Compress
    try {
        $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($body)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s
        return @{ Status = [int]$r.StatusCode; S = $s; Body = $r.Content }
    } catch {
        $code = 0
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        return @{ Status = $code; S = $null; Body = "" }
    }
}

$admin = New-Session
$qaEmail = "qa-auth-$rnd@example.com"
$nu = Api $admin POST "/users" @{ email = $qaEmail; password = "QaUser2026!"; display_name = "QA Auth" }
"qa user: $qaEmail create=$($nu.Status)"

# A1 正确密码登录，种下会话 cookie
$a1 = Invoke-Login -Email $qaEmail -Pw "QaUser2026!"
$cookieCount = 0
if ($null -ne $a1.S) { $cookieCount = $a1.S.Cookies.GetCookies($Base).Count }
Check A1 "正确密码登录且种下会话cookie" ($a1.Status -eq 200 -and $cookieCount -ge 1) "status=$($a1.Status) cookies=$cookieCount"

# A2 错误密码登录 401，不种会话
$a2 = Invoke-Login -Email $qaEmail -Pw "WrongPass$rnd!"
$a2NoSess = ($null -eq $a2.Session)
Check A2 "错误密码登录 401 且无会话" ($a2.Status -eq 401 -and $a2NoSess) "status=$($a2.Status) nosession=$a2NoSess"

# A3 登录后 GET /sso/me 返回正确 email
$a3 = Api $a1 GET "/sso/me"
$a3Email = $null
if ($a3.Data) { $a3Email = $a3.Data.email }
Check A3 "GET /sso/me 返回登录邮箱" ($a3.Status -eq 200 -and $a3Email -eq $qaEmail) "status=$($a3.Status) email=$a3Email"

# A4 PUT /me/settings 改签名/显示名，再读生效
$disp = "QA显示名$rnd"
$sig = "QA签名-$rnd"
$a4put = Api $a1 PUT "/me/settings" @{ displayed_name = $disp; signature = $sig }
$a4me = Api $a1 GET "/me"
$a4sso = Api $a1 GET "/sso/me"
$back1 = ""; $back2 = ""
if ($a4me.Data) { $back1 = $a4me.Data.displayed_name }
if ($a4sso.Data) { $back2 = $a4sso.Data.displayed_name }
Check A4 "改签名/显示名后再读生效" ($a4put.Status -in @(200,204) -and $back1 -eq $disp -and $back2 -eq $disp) "put=$($a4put.Status) me=$back1 sso=$back2"

# A5 改密码链路：改→旧密码失败→新密码成功→改回→原密码成功
$a5chg = Api $a1 PUT "/me/password" @{ old_pw = "QaUser2026!"; new_pw = "QaNew2026!" }
$a5old = Invoke-Login -Email $qaEmail -Pw "QaUser2026!"
$a5new = Invoke-Login -Email $qaEmail -Pw "QaNew2026!"
$a5back = $null
if ($a5new.S) { $a5back = Api $a5new PUT "/me/password" @{ old_pw = "QaNew2026!"; new_pw = "QaUser2026!" } }
$a5re = Invoke-Login -Email $qaEmail -Pw "QaUser2026!"
Check A5 "改密码→旧失败→新成功→改回" ($a5chg.Status -eq 204 -and $a5old.Status -eq 401 -and $a5new.Status -eq 200 -and $a5back.Status -eq 204 -and $a5re.Status -eq 200) "chg=$($a5chg.Status) old=$($a5old.Status) new=$($a5new.Status) back=$($a5back.Status) re=$($a5re.Status)"

# A6 POST /sso/logout 后 /sso/me 401
$a6l = Invoke-Login -Email $qaEmail -Pw "QaUser2026!"
$a6out = Api $a6l POST "/sso/logout" @{}
$a6me = Api $a6l GET "/sso/me"
Check A6 "logout 后 /sso/me 401" ($a6out.Status -in @(200,204) -and $a6me.Status -eq 401) "logout=$($a6out.Status) me=$($a6me.Status)"

# A5 改密码后的新会话（改密可能吊销旧会话，A7/A8 用新会话）
$a7sess = $a1
if ($null -ne $a5re.S) { $a7sess = $a5re }

# A7 GET /me/logins 返回数组含本机登录
$a7 = Api $a7sess GET "/me/logins"
$a7n = 0
if ($a7.Data) { $a7n = @($a7.Data).Count }
Check A7 "GET /me/logins 有本机登录记录" ($a7.Status -eq 200 -and $a7n -ge 1) "status=$($a7.Status) rows=$a7n"

# A8 App Token 建/列/删
$t = Api $a7sess POST "/tokens" @{ ip = "qa-batch-a" }
$tid = $null
if ($t.Data) { $tid = $t.Data.id }
$tl = Api $a7sess GET "/tokens?page=1&limit=50"
$tlHit = $false
if ($tl.Data -and $tl.Data.data) { $tlHit = (@($tl.Data.data | Where-Object { "$($_.id)" -eq "$tid" }).Count -ge 1) }
$tdel = $null
if ($tid) { $tdel = Api $a7sess DELETE "/tokens/$tid" }
$tl2 = Api $a7sess GET "/tokens?page=1&limit=50"
$tlGone = $true
if ($tl2.Data -and $tl2.Data.data) { $tlGone = (@($tl2.Data.data | Where-Object { "$($_.id)" -eq "$tid" }).Count -eq 0) }
Check A8 "App Token 建/列/删" ($t.Status -eq 201 -and $tid -ne $null -and $tl.Status -eq 200 -and $tlHit -and $tdel.Status -eq 204 -and $tlGone) "create=$($t.Status) id=$tid list=$tlHit del=$($tdel.Status) gone=$tlGone"

Flush-Results "A"
"== done =="
