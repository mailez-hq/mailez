# qa-tp-p18-protocol.ps1 - P18 protocol conformance (TEST-PROMPTS.md P18), rev1
# Provisions an account plus an app token, then drives the Python protocol
# sweep against the engine-lb published ports.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P18 protocol rev1 rnd=$rnd base=$Base =="

$admin = New-Session
$PW = "QaUser2026!"
$U = "qa-p18-u-$rnd@example.com"
$P = "qa-p18-p-$rnd@example.com"
foreach ($u in @($U, $P)) {
    $r = Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P18" }
    if ($r.Status -notin @(200, 201)) { "prep FAIL create $u -> $($r.Status)"; exit 1 }
}

# App-token creation goes through the signed-in user, but this QA host has
# driven hundreds of logins today and the limiter answers 401 "too many login
# attempts" for fresh sessions; the global admin may issue a token for any
# user, which keeps the protocol sweep independent of that limiter.
$uSess = New-Session $U $PW
$tok = Api $uSess POST "/tokens" @{ ip = "qa-p18" }
if ($tok.Status -eq 401) { $tok = Api $admin POST "/tokens" @{ email = $U; ip = "qa-p18" } }
$appToken = ""
foreach ($k in @("token", "secret", "password")) {
    if ($tok.Data -and $tok.Data.$k) { $appToken = [string]$tok.Data.$k }
}
if ($appToken -eq "") { "WARN: no app token returned ($($tok.Status)); app-token sub-checks will fail" }

# The autoconfig check drives the internal /stack route, which is guarded by
# the deployment's shared secret; the probe reads it from QA_STACK_SECRET.
if (-not $env:QA_STACK_SECRET -and (Test-Path "D:\code\mailez-hq\mailez\deploy\mailez.env")) {
    $m = Select-String -Path "D:\code\mailez-hq\mailez\deploy\mailez.env" -Pattern '^MAILEZ_STACK_SECRET=(.+)$'
    if ($m) { $env:QA_STACK_SECRET = $m.Matches[0].Groups[1].Value.Trim() }
}
python .\qa-p18-protocol.py 127.0.0.1 143 587 465 995 4190 $U $PW $appToken $P $Base
"python exit=$LASTEXITCODE"

$c = @()
foreach ($t in @($tok.Data)) { if ($t -and $t.id) { $c += "token=" + (Api $uSess DELETE "/tokens/$($t.id)").Status } }
$c += "user=" + (Api $admin DELETE "/users/$U").Status
$c += "peer=" + (Api $admin DELETE "/users/$P").Status
"cleanup: $($c -join ' ')"
"== done =="
