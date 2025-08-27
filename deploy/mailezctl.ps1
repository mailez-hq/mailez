# mailezctl — 两档管理入口：开发 / 生产。
#
# 只有两个 compose 文件，对应两档存储：
#   dev  = docker-compose.dev.yml   （SQLite + Pebble + 本地 FS）
#   prod = docker-compose.prod.yml  （MySQL + TiDB + MinIO/S3）
#
# 用法:
#   powershell .\deploy\mailezctl.ps1 up              # 开发档
#   powershell .\deploy\mailezctl.ps1 up prod         # 生产档
#   powershell .\deploy\mailezctl.ps1 ps
#   powershell .\deploy\mailezctl.ps1 logs prod mailezine -Follow
#   powershell .\deploy\mailezctl.ps1 down

param(
    [Parameter(Position = 0)]
    [ValidateSet("up", "down", "ps", "logs", "build", "config")]
    [string]$Action = "ps",

    [Parameter(Position = 1)]
    [ValidateSet("dev", "prod")]
    [string]$Target = "dev",

    [Parameter(Position = 2)]
    [string]$Service = "",

    [switch]$Follow
)

$ErrorActionPreference = "Stop"
$deploy = Split-Path -Parent $MyInvocation.MyCommand.Path
$file = if ($Target -eq "prod") { "docker-compose.prod.yml" } else { "docker-compose.dev.yml" }

$compose = @("docker", "compose", "-f", $file)
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
