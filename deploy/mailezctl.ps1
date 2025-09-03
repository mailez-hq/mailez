# mailezctl — 管理入口：开发 / 社区版 / 企业版 / HA。
#
# 三个自包含 compose 文件：
#   dev  = docker-compose.dev.yml   （SQLite + Pebble + 本地 FS）
#   ce  = docker-compose.ce.yml（社区版：mailezine + SQLite 控制面）
#   ee  = ee/docker-compose.ee.yml（企业版：mailezine + MySQL +
#                TiDB + MinIO/S3）
# 外加叠加档：
#   ha = ee + ee/docker-compose.ha.yml（backend/frontend 多副本）
#   multi = ee + ee/docker-compose.multi.yml（引擎多活）
#
# 用法:
#   powershell .\deploy\mailezctl.ps1 up              # 开发档
#   powershell .\deploy\mailezctl.ps1 up ce    # 社区版生产
#   powershell .\deploy\mailezctl.ps1 up ee   # 企业版生产
#   powershell .\deploy\mailezctl.ps1 up ha           # 企业版 + 控制面多副本
#   powershell .\deploy\mailezctl.ps1 up multi        # 企业版 + 引擎多活
#   powershell .\deploy\mailezctl.ps1 ps
#   powershell .\deploy\mailezctl.ps1 logs ee mailezine -Follow
#   powershell .\deploy\mailezctl.ps1 down

param(
    [Parameter(Position = 0)]
    [ValidateSet("up", "down", "ps", "logs", "build", "config")]
    [string]$Action = "ps",

    [Parameter(Position = 1)]
    [ValidateSet("dev", "ce", "ee", "ha", "multi")]
    [string]$Target = "dev",

    [Parameter(Position = 2)]
    [string]$Service = "",

    [switch]$Follow
)

$ErrorActionPreference = "Stop"
$deploy = Split-Path -Parent $MyInvocation.MyCommand.Path
$files = switch ($Target) {
    "ce"  { ,@("docker-compose.ce.yml") }
    "ee"  { ,@("ee/docker-compose.ee.yml") }
    "ha"         { ,@("ee/docker-compose.ee.yml", "ee/docker-compose.ha.yml") }
    "multi"      { ,@("ee/docker-compose.ee.yml", "ee/docker-compose.multi.yml") }
    default      { ,@("docker-compose.dev.yml") }
}

# 企业版配方位于 ee/（私有树，社区镜像中已剥离）。给出友好提示而非
# compose 缺文件报错。
if (-not (Test-Path (Join-Path $deploy $files[0]))) {
    Write-Error "profile '$Target' is not part of this tree (ee/ deployment recipes are enterprise-only)"
    exit 2
}

$compose = @("docker", "compose", "--env-file", "mailez.env")
foreach ($f in $files) { $compose += @("-f", $f) }
switch ($Action) {
    "up"     { $compose += @("up", "-d", "--build") }
    "down"   { $compose += @("down") }
    "ps"     { $compose += @("ps") }
    "logs"   { $compose += @("logs"); if ($Follow) { $compose += "--follow" } }
    "build"  { $compose += @("build") }
    "config" { $compose += @("config") }
}
if ($Service) { $compose += $Service }

Write-Host ("== mailezctl {0} ({1})" -f $Action, $Target)
Push-Location $deploy
try {
    & $compose[0] $compose[1..($compose.Count - 1)]
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}
