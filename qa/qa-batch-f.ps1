# qa-batch-f.ps1 — F1..F6 设置与账户安全
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== F 设置与账户安全 rnd=$rnd =="
# 与 qa-lib 相同的 Base 解析（QA_BASE 优先），供自定义 Header 请求使用
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
$s = New-Session
$qa = "qa-sec-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $qa; password = "QaUser2026!"; display_name = "QA Sec" }
$qs = New-Session -Email $qa -Pw "QaUser2026!"

# F1 PGP：生成密钥→/pgp/key 命中→加密/解密/签名/验签闭环
$gen = Api $qs POST "/me/pgp/generate" @{}
$pub = ""; $fp = ""
if ($gen.Data) { $pub = $gen.Data.public_key; $fp = $gen.Data.fingerprint }
$st = Api $qs GET "/me/pgp"
$lookup = Api $s GET "/pgp/key?email=$([uri]::EscapeDataString($qa))"
$enc = Api $s POST "/mail/pgp/encrypt" @{ text = "qa f1 secret $rnd"; public_key = $pub }
$cipher = ""
if ($enc.Data) { $cipher = $enc.Data.encrypted }
$dec = $null
$plain = ""
if ($cipher -ne "") {
    $dec = Api $qs POST "/mail/pgp/decrypt" @{ text = $cipher }
    if ($dec.Data) { $plain = $dec.Data.plaintext }
}
$sign = Api $qs POST "/mail/pgp/sign" @{ text = "qa f1 signed $rnd" }
$sig = ""
if ($sign.Data) { $sig = $sign.Data.signature }
$ver = Api $s POST "/mail/pgp/verify" @{ text = "qa f1 signed $rnd"; signature = $sig; public_key = $pub }
$verOk = $false
if ($ver.Data) { $verOk = ($ver.Data.valid -eq $true) }
Check F1 "PGP 生成/查询/加解密/签名验签" ($gen.Status -eq 200 -and $pub -ne "" -and $st.Status -eq 200 -and $lookup.Status -eq 200 -and $enc.Status -eq 200 -and $dec.Status -eq 200 -and $plain -eq "qa f1 secret $rnd" -and $verOk) "gen=$($gen.Status) fp=$fp lookup=$($lookup.Status) enc=$($enc.Status) dec=$($dec.Status) plain_ok=$($plain -eq "qa f1 secret $rnd") ver=$verOk"

# F2 S/MIME：本机自签证书 → p12 导入 → /smime/cert 查询 → 加解密 → 清理
$f2Note = ""
$cert = $null
$f2CertOk = $false
try {
    Import-Module PKI -ErrorAction Stop
    $cert = New-SelfSignedCertificate -Subject "CN=qa-f2-$rnd" -DnsName "qa-f2-$rnd.example.com" -CertStoreLocation "Cert:\CurrentUser\My" -KeyExportPolicy Exportable -KeySpec KeyExchange -KeyUsage DigitalSignature,KeyEncipherment,DataEncipherment -KeyLength 2048 -NotAfter ([DateTime]::Now.AddYears(1)) -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.4") -ErrorAction Stop
    $f2CertOk = ($cert -ne $null)
} catch {
    $f2Note = "local-cert-fail: " + $_.Exception.Message
}
$pfxB64 = ""
$p12Pass = "P12pass$rnd!"
if ($f2CertOk) {
    $tmpPfx = Join-Path $env:TEMP "qa-f2-$rnd.pfx"
    try {
        $sec = ConvertTo-SecureString -String $p12Pass -Force -AsPlainText
        $null = Export-PfxCertificate -Cert $cert -FilePath $tmpPfx -Password $sec -ErrorAction Stop
        $pfxB64 = [Convert]::ToBase64String([System.IO.File]::ReadAllBytes($tmpPfx))
        Remove-Item $tmpPfx -Force -ErrorAction SilentlyContinue
    } catch {
        $f2Note = "pfx-export-fail: " + $_.Exception.Message
    }
}
$f2imp = $null
if ($pfxB64 -ne "") { $f2imp = Api $qs POST "/me/smime" @{ p12_b64 = $pfxB64; p12_password = $p12Pass } }
$f2st = Api $qs GET "/me/smime"
$f2Has = $false
if ($f2st.Data) { $f2Has = ($f2st.Data.has_cert -eq $true) }
$f2certLookup = Api $s GET "/smime/cert?email=$([uri]::EscapeDataString($qa))"
$f2CertPem = ""
if ($f2certLookup.Data) { $f2CertPem = $f2certLookup.Data.cert_pem }
$f2enc = $null; $f2cipher = ""
if ($f2CertPem -ne "") {
    $f2enc = Api $s POST "/mail/smime/encrypt" @{ text = "qa f2 secret $rnd"; cert_pem = $f2CertPem }
    if ($f2enc.Data) { $f2cipher = $f2enc.Data.encrypted }
}
$f2dec = $null; $f2plain = ""
if ($f2cipher -ne "") {
    $f2dec = Api $qs POST "/mail/smime/decrypt" @{ text = $f2cipher }
    if ($f2dec.Data) { $f2plain = $f2dec.Data.plaintext }
}
$f2del = Api $qs DELETE "/me/smime"
if ($f2CertOk) { Remove-Item "Cert:\CurrentUser\My\$($cert.Thumbprint)" -Force -ErrorAction SilentlyContinue }
Check F2 "S/MIME 证书导入/查询/加解密" ($f2imp -ne $null -and $f2imp.Status -eq 200 -and $f2st.Status -eq 200 -and $f2Has -and $f2certLookup.Status -eq 200 -and $f2enc.Status -eq 200 -and $f2dec.Status -eq 200 -and $f2plain -eq "qa f2 secret $rnd" -and $f2del.Status -eq 204) "imp=$($f2imp.Status) has=$f2Has lookup=$($f2certLookup.Status) enc=$($f2enc.Status) dec=$($f2dec.Status) del=$($f2del.Status) $f2Note"

# F3 委托 CRUD；被委托人会话可见主人邮箱
$dep = Api $s POST "/delegations" @{ delegate_email = $qa; can_send = $true; full_access = $true }
$depId = $null
if ($dep.Data) { $depId = $dep.Data.id }
$depList = Api $s GET "/delegations"
$depHit = $false
if ($depList.Data -and $depList.Data.granted) { $depHit = (@($depList.Data.granted | Where-Object { $_.delegate_email -eq $qa }).Count -ge 1) }
$recv = Api $qs GET "/delegations"
$recvHit = $false
if ($recv.Data -and $recv.Data.received) { $recvHit = (@($recv.Data.received | Where-Object { $_.owner_email -eq "admin@example.com" }).Count -ge 1) }
$idsAsDep = Api $qs GET "/mail/identities"
$depIdent = $false
if ($idsAsDep.Data) { $depIdent = (@($idsAsDep.Data | Where-Object { $_.email -eq "admin@example.com" -and $_.delegated }).Count -ge 1) }
# 被委托人以 X-Delegate-Email 读主人邮箱
$delFolders = 0
try {
    $r = Invoke-WebRequest -Uri "$Base/mail/folders" -Method GET -WebSession $qs.S -Headers @{ "X-Delegate-Email" = "admin@example.com" } -UseBasicParsing -TimeoutSec 60
    $delFolders = [int]$r.StatusCode
} catch {
    if ($_.Exception.Response) { $delFolders = [int]$_.Exception.Response.StatusCode }
}
$depUp = $null
if ($depId) { $depUp = Api $s PUT "/delegations/$depId" @{ delegate_email = $qa; can_send = $true; full_access = $false } }
$depDel = $null
if ($depId) { $depDel = Api $s DELETE "/delegations/$depId" }
Check F3 "委托 CRUD + 被委托人可见主人邮箱" ($dep.Status -eq 200 -and $depId -ne $null -and $depHit -and $recvHit -and $depIdent -and $delFolders -eq 200 -and $depUp.Status -eq 200 -and $depDel.Status -eq 204) "c=$($dep.Status) granted=$depHit received=$recvHit ident=$depIdent folders=$delFolders up=$($depUp.Status) del=$($depDel.Status)"

# F4 Sieve 脚本 存/激活/取/删
$sName = "qa-f4-$rnd"
$sContent = "require [`"fileinto`"];`r`n# qa-f4 rule $rnd`r`nif header :contains `"Subject`" `"qa-f4`" { fileinto `"Junk`"; stop; }`r`n"
$svPut = Api $qs PUT "/sieve/$sName" @{ content = $sContent; activate = $true }
$svList = Api $qs GET "/sieve"
$svHit = $false
if ($svList.Data) { $svHit = (@($svList.Data | Where-Object { $_.name -eq $sName }).Count -ge 1) }
$svGet = Api $qs GET "/sieve/$sName"
$svContentOk = $false
if ($svGet.Data) { $svContentOk = ("$($svGet.Data.content)" -match [regex]::Escape("qa-f4 rule $rnd")) }
$svAct = Api $qs POST "/sieve/$sName/activate" @{}
$svDel = Api $qs DELETE "/sieve/$sName"
Check F4 "Sieve 存/列/取/激活/删" ($svPut.Status -eq 204 -and $svHit -and $svGet.Status -eq 200 -and $svContentOk -and $svAct.Status -eq 204 -and $svDel.Status -eq 204) "put=$($svPut.Status) list=$svHit get=$($svGet.Status)/$svContentOk act=$($svAct.Status) del=$($svDel.Status)"

# F5 Webhooks CRUD + test
$hookUrl = "$Base/health"
$wh = Api $qs POST "/webhooks" @{ url = $hookUrl; secret = "qa-f5-secret"; events = "mail.received" }
$whId = $null
if ($wh.Data) { $whId = $wh.Data.id }
$whList = Api $qs GET "/webhooks"
$whHit = $false
if ($whList.Data) { $whHit = (@($whList.Data | Where-Object { "$($_.id)" -eq "$whId" }).Count -ge 1) }
$whUp = $null
if ($whId) { $whUp = Api $qs PUT "/webhooks/$whId" @{ enabled = $false } }
$whTest = $null
$whTestOk = $false
if ($whId) {
    $whTest = Api $qs POST "/webhooks/$whId/test" @{}
    if ($whTest.Data) { $whTestOk = ($whTest.Data.PSObject.Properties.Name -contains "status") }
}
$whDel = $null
if ($whId) { $whDel = Api $qs DELETE "/webhooks/$whId" }
Check F5 "Webhooks 建/列/改/test/删" ($wh.Status -eq 200 -and $whId -ne $null -and $whHit -and $whUp.Status -eq 200 -and $whTest.Status -eq 200 -and $whTestOk -and $whDel.Status -in @(200,204)) "c=$($wh.Status) list=$whHit up=$($whUp.Status) test=$($whTest.Status) struct=$whTestOk del=$($whDel.Status)"

# F6 TOTP 2FA 全流程：开→登录要验证码→验证登录→关
# RFC 6238 本地实现（Base32 解码 + HMACSHA1，30s 步长，6 位）
function ConvertFrom-Base32 {
    param([string]$Value)
    $alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
    $bits = ""
    foreach ($ch in $Value.ToUpper().ToCharArray()) {
        $idx = $alpha.IndexOf($ch)
        if ($idx -ge 0) { $bits += [Convert]::ToString($idx, 2).PadLeft(5, "0") }
    }
    $n = [int][math]::Floor($bits.Length / 8)
    $out = New-Object byte[] $n
    for ($i = 0; $i -lt $n; $i++) { $out[$i] = [Convert]::ToByte($bits.Substring($i * 8, 8), 2) }
    return ,$out
}
function Get-TotpCode {
    param([string]$Secret, [int]$StepOffset = 0)
    $unix = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    $counter = [UInt64]([math]::Floor($unix / 30) + $StepOffset)
    $buf = [BitConverter]::GetBytes($counter)
    if ([BitConverter]::IsLittleEndian) { [array]::Reverse($buf) }
    $key = ConvertFrom-Base32 $Secret
    $hmac = New-Object System.Security.Cryptography.HMACSHA1 -ArgumentList (,$key)
    $sum = $hmac.ComputeHash($buf)
    $off = $sum[$sum.Length - 1] -band 0x0f
    $bin = (([int]$sum[$off] -band 0x7f) -shl 24) -bor (([int]$sum[$off + 1] -band 0xff) -shl 16) -bor (([int]$sum[$off + 2] -band 0xff) -shl 8) -bor ([int]$sum[$off + 3] -band 0xff)
    return ($bin % 1000000).ToString("D6")
}
$f6st = Api $qs GET "/me/totp"
$f6secret = ""
if ($f6st.Data) { $f6secret = $f6st.Data.secret }
$code1 = ""
if ($f6secret -ne "") { $code1 = Get-TotpCode -Secret $f6secret }
$f6en = $null
if ($code1 -ne "") { $f6en = Api $qs POST "/me/totp/enable" @{ code = $code1 } }
# 开启后普通登录应要求验证码（200 + totp_required，不发会话 cookie）
$f6loginBody = @{ email = $qa; pw = "QaUser2026!" } | ConvertTo-Json -Compress
$f6TotpReq = $false; $f6Pending = ""
try {
    $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($f6loginBody)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s6
    $f6login = [Text.Encoding]::UTF8.GetString($r.RawContentStream.ToArray()) | ConvertFrom-Json
    if ($f6login.totp_required -eq $true) { $f6TotpReq = $true; $f6Pending = $f6login.pending_token }
} catch {}
$code2 = ""
if ($f6secret -ne "") { $code2 = Get-TotpCode -Secret $f6secret }
$f6ver = $null
$f6verOk = $false
if ($f6Pending -ne "" -and $code2 -ne "") {
    $f6ver = Api (New-Object -TypeName psobject -Property @{ S = $s6 }) POST "/sso/login/totp" @{ pending_token = $f6Pending; code = $code2 } -Expect 200
    if ($f6ver.Status -eq 200) { $f6verOk = $true }
}
# 关闭 2FA（验证当前码），再普通登录直通
$code3 = ""
if ($f6secret -ne "") { $code3 = Get-TotpCode -Secret $f6secret }
$f6dis = $null
if ($f6verOk) { $f6dis = Api $qs DELETE "/me/totp" @{ code = $code3 } }
$f6plain = 0
try {
    $r2 = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body ([Text.Encoding]::UTF8.GetBytes($f6loginBody)) -ContentType "application/json" -UseBasicParsing -TimeoutSec 40 -SessionVariable s6b
    $f6plain = [int]$r2.StatusCode
} catch {
    if ($_.Exception.Response) { $f6plain = [int]$_.Exception.Response.StatusCode }
}
Check F6 "TOTP 开启→登录要码→验证登录→关闭" ($f6st.Status -eq 200 -and $f6secret -ne "" -and $f6en.Status -eq 204 -and $f6TotpReq -and $f6verOk -and $f6dis.Status -eq 204 -and $f6plain -eq 200) "secret=$($f6secret -ne $null) en=$($f6en.Status) totpReq=$f6TotpReq verLogin=$f6verOk dis=$($f6dis.Status) plainLogin=$f6plain"

Flush-Results "F"
"== done =="
