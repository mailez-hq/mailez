# mailezctl — 管理入口：开发 / 社区版 / 企业版。
#
# 三个自包含 compose 文件：
#   dev  = docker-compose.dev.yml   （SQLite + Pebble + 本地 FS）
#   community = docker-compose.community.yml（社区版：mailezine + MySQL）
#   enterprise = docker-compose.enterprise.yml（企业版：mailezine + MySQL +
#                TiDB + MinIO/S3）
#
# 用法:
#   powershell .\deploy\mailezctl.ps1 up              # 开发档
#   powershell .\deploy\mailezctl.ps1 up community    # 社区版生产
#   powershell .\deploy\mailezctl.ps1 up enterprise   # 企业版生产
#   powershell .\deploy\mailezctl.ps1 ps
#   powershell .\deploy\mailezctl.ps1 logs enterprise mailezine -Follow
#   powershell .\deploy\mailezctl.ps1 down

param(
    [Parameter(Position = 0)]
    [ValidateSet("up", "down", "ps", "logs", "build", "config")]
    [string]$Action = "ps",

    [Parameter(Position = 1)]
    [ValidateSet("dev", "community", "enterprise")]
    [string]$Target = "dev",

    [Parameter(Position = 2)]
    [string]$Service = "",

    [switch]$Follow
)

$ErrorActionPreference = "Stop"
$deploy = Split-Path -Parent $MyInvocation.MyCommand.Path
$file = switch ($Target) {
    "community"  { "docker-compose.community.yml" }
    "enterprise" { "docker-compose.enterprise.yml" }
    default      { "docker-compose.dev.yml" }
}

$compose = @("docker", "compose", "--env-file", "mailez.env", "-f", $file)
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
