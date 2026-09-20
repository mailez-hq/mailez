# qa-auth-phantom.ps1 - quantify the intermittent 401 on correct credentials
# (OBS-AUTH-PHANTOM): 25 logins with the known-good admin password, recording
# status + latency, plus a snapshot of the auth-related Redis keys.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://127.0.0.1:8081/api/v1" }
"== auth phantom probe =="
$body = @{ email = "admin@example.com"; pw = "MailezDemo2026!" } | ConvertTo-Json -Compress
$bytes = [Text.Encoding]::UTF8.GetBytes($body)
$ok = 0; $fail = 0; $latOk = @(); $latFail = @(); $details = @()
for ($i = 1; $i -le 25; $i++) {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $code = 0
    try {
        $r = Invoke-WebRequest -Uri "$Base/sso/login" -Method POST -Body $bytes -ContentType "application/json" -UseBasicParsing -TimeoutSec 30
        $code = [int]$r.StatusCode
    } catch {
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode } else { $code = -1 }
    }
    $ms = [int]$sw.Elapsed.TotalMilliseconds
    if ($code -eq 200) { $ok++; $latOk += $ms } else { $fail++; $latFail += $ms; $details += "attempt$i=$code/${ms}ms" }
    Start-Sleep -Milliseconds 250
}
"logins: ok=$ok fail=$fail"
if ($latOk.Count) { "ok latency ms: min=$([int]($latOk | Measure-Object -Minimum).Minimum) median=$([int]($latOk | Sort-Object)[[int]($latOk.Count/2)]) max=$([int]($latOk | Measure-Object -Maximum).Maximum)" }
if ($latFail.Count) { "fail latency ms: min=$([int]($latFail | Measure-Object -Minimum).Minimum) max=$([int]($latFail | Measure-Object -Maximum).Maximum) -> [$($details -join ' ')]" }
$redis = docker exec mailez-redis-1 redis-cli --scan --pattern "mailez:*" 2>&1 | Select-Object -First 20
"redis keys: $($redis -join ', ')"
$ac = docker exec mailez-redis-1 redis-cli --scan --pattern "*authcache*" 2>&1 | Select-Object -First 5
"authcache keys: $($ac -join ', ')"
