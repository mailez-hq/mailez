# mailezctl — 统一编排管理入口。
#
# 一个命令管理整套栈（mailez 控制面 + 邮件引擎），引擎与存储通过 profile
# 切换，compose 文件全部属于同一个 "mailez" 项目：
#
#   Engine          Compose 文件                                    Profile
#   --------------- ----------------------------------------------  ----------------
#   postdove        docker-compose.dev.yml                          (默认)
#   mailezine       dev + docker-compose.mailezine.yml              --profile mailezine
#   mailezine-tidb  dev + mailezine + docker-compose.tidb.yml       --profile mailezine
#
# 用法:
#   powershell .\deploy\mailezctl.ps1 up mailezine-tidb
#   powershell .\deploy\mailezctl.ps1 up mailezine
#   powershell .\deploy\mailezctl.ps1 ps
#   powershell .\deploy\mailezctl.ps1 logs mailezine -Follow
#   powershell .\deploy\mailezctl.ps1 down
#
# -Prod 切换基座为 docker-compose.yml（完整容器化，backend/frontend 也在
# 容器内）；默认 dev 基座要求后端在宿主机 8080 运行（见 docs/dev-setup.md）。

param(
    [Parameter(Position = 0)]
    [ValidateSet("up", "down", "ps", "logs", "build", "config")]
    [string]$Action = "ps",

    [Parameter(Position = 1)]
    [ValidateSet("postdove", "mailezine", "mailezine-tidb")]
    [string]$Engine = "postdove",

    [Parameter(Position = 2)]
    [string]$Service = "",
    [switch]$Follow,
    [switch]$Prod
)

$ErrorActionPreference = "Stop"
$deploy = Split-Path -Parent $MyInvocation.MyCommand.Path

$base = if ($Prod) { "docker-compose.yml" } else { "docker-compose.dev.yml" }
$files = @("-f", $base)
$profile = $null

switch ($Engine) {
    "mailezine" {
        $files += @("-f", "docker-compose.mailezine.yml")
        $profile = "mailezine"
    }
    "mailezine-tidb" {
        $files += @("-f", "docker-compose.mailezine.yml", "-f", "docker-compose.tidb.yml")
        $profile = "mailezine"
    }
}

$compose = @("docker", "compose")
$compose += $files
if ($profile) { $compose += @("--profile", $profile) }

switch ($Action) {
    "up" {
        $compose += @("up", "-d", "--build")
        if ($Service) { $compose += $Service }
    }
    "down" {
        $compose += @("down")
        if ($Service) { $compose += $Service }
    }
    "ps" { $compose += @("ps") }
    "logs" {
        $compose += @("logs")
        if ($Follow) { $compose += "--follow" }
        if ($Service) { $compose += $Service }
    }
    "build" {
        $compose += @("build")
        if ($Service) { $compose += $Service }
    }
    "config" { $compose += @("config") }
}

Write-Host ("== mailezctl {0} (engine={1}, prod={2})" -f $Action, $Engine, $Prod)
Push-Location $deploy
try {
    & $compose[0] $compose[1..($compose.Count - 1)]
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}
