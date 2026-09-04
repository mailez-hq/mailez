# mailezctl — 管理入口：开发 / 生产（ce）。
#
# 两个自包含 compose 文件：
#   dev = docker-compose.dev.yml （SQLite + Pebble + 本地 FS）
#   ce  = docker-compose.ce.yml  （mailezine + SQLite 控制面）
#
# 用法:
#   powershell .\deploy\mailezctl.ps1 up              # 开发档
#   powershell .\deploy\mailezctl.ps1 up ce           # 生产（拉镜像）
#   powershell .\deploy\mailezctl.ps1 ps ce
#   powershell .\deploy\mailezctl.ps1 logs ce backend -Follow
#   powershell .\deploy\mailezctl.ps1 down ce

param(
    [Parameter(Position = 0)]
    [ValidateSet("up", "down", "ps", "logs", "build", "config")]
    [string]$Action = "ps",

    [Parameter(Position = 1)]
    [ValidateSet("dev", "ce")]
    [string]$Target = "dev",

    [Parameter(Position = 2)]
    [string]$Service = "",

    [switch]$Follow
)

$ErrorActionPreference = "Stop"
$deploy = Split-Path -Parent $MyInvocation.MyCommand.Path
$files = switch ($Target) {
    "ce" { ,@("docker-compose.ce.yml") }
    default { ,@("docker-compose.dev.yml") }
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
