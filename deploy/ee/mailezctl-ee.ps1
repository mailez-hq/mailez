# mailezctl-ee — 私有树管理入口：开发 / ce / ee / ha / multi。
#
# 不导出到公仓源码树。公仓的 mailezctl 只认识 dev / ce 两档；本配套脚本
# 补充 deploy/ee/ 下的扩展部署配方：
#   ee    = ee/docker-compose.ee.yml        （mailezine + MySQL + TiDB + MinIO/S3）
#   ha    = ee + ee/docker-compose.ha.yml   （控制面多副本）
#   multi = ee + ee/docker-compose.multi.yml（引擎多活）
#
# 用法:
#   powershell .\deploy\ee\mailezctl-ee.ps1 up ee     # 生产（拉镜像）
#   powershell .\deploy\ee\mailezctl-ee.ps1 up ha
#   powershell .\deploy\ee\mailezctl-ee.ps1 up multi
#   powershell .\deploy\ee\mailezctl-ee.ps1 ps ee
#   powershell .\deploy\ee\mailezctl-ee.ps1 logs ee backend -Follow
#   powershell .\deploy\ee\mailezctl-ee.ps1 down ee

param(
    [Parameter(Position = 0)]
    [ValidateSet("up", "down", "ps", "logs", "build", "config")]
    [string]$Action = "ps",

    [Parameter(Position = 1)]
    [ValidateSet("dev", "ce", "ee", "ha", "multi")]
    [string]$Target = "ee",

    [Parameter(Position = 2)]
    [string]$Service = "",

    [switch]$Follow
)

$ErrorActionPreference = "Stop"
# deploy/ee/ 下的本脚本 → 上一级即 deploy/
$deploy = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$files = switch ($Target) {
    "ce"    { ,@("docker-compose.ce.yml") }
    "ee"    { ,@("ee/docker-compose.ee.yml") }
    "ha"    { ,@("ee/docker-compose.ee.yml", "ee/docker-compose.ha.yml") }
    "multi" { ,@("ee/docker-compose.ee.yml", "ee/docker-compose.multi.yml") }
    default { ,@("docker-compose.dev.yml") }
}

foreach ($f in $files) {
    if (-not (Test-Path (Join-Path $deploy $f))) {
        Write-Error "missing compose file: $f (run from the private tree)"
        exit 2
    }
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

Write-Host ("== mailezctl-ee {0} ({1})" -f $Action, $Target)
Push-Location $deploy
try {
    & $compose[0] $compose[1..($compose.Count - 1)]
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}
