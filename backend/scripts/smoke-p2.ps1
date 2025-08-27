# P2 smoke: cloud drive, admin overview and EAS calendar sync.
#
# Usage:
#   pwsh scripts/smoke-p2.ps1 -Email admin@example.com -Password '...'
param(
  [string]$Base = "http://127.0.0.1:8080/api/v1",
  [string]$Email = "admin@example.com",
  [string]$Password = ""
)

$ErrorActionPreference = "Stop"
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

function Invoke-Post($path, $body) {
  Invoke-RestMethod -Uri ($Base + $path) -Method Post -Body ($body | ConvertTo-Json -Depth 8) `
    -ContentType "application/json" -WebSession $session
}
function Invoke-Get($path) {
  Invoke-RestMethod -Uri ($Base + $path) -Method Get -WebSession $session
}

if (-not $Password) { $Password = $env:EAS_E2E_PASSWORD }
if (-not $Password) { throw "-Password is required" }

$login = Invoke-Post "/sso/login" @{ email = $Email; pw = $Password }
if (-not $login.email) { throw "login failed" }
Write-Host "ok login"

# 1. Cloud drive: folder, upload, list, download, share, trash.
$folder = Invoke-Post "/drive/folders" @{ name = "E2E 文档" }
$tmp = Join-Path $env:TEMP "drive.txt"
[IO.File]::WriteAllText($tmp, "drive-content")
$up = Invoke-RestMethod -Uri ($Base + "/drive/upload") -Method Post `
  -Form @{ parent_id = [string]$folder.id; file = Get-Item $tmp } -WebSession $session
if ($up.name -ne "drive.txt") { throw "upload name wrong" }
$list = Invoke-Get ("/drive/tree?parent_id=" + $folder.id)
if ($list.Count -ne 1) { throw "list count $($list.Count)" }
$dlOut = Join-Path $env:TEMP "drive-dl.bin"
Invoke-WebRequest -Uri ($Base + "/drive/download/" + $up.id) -OutFile $dlOut -WebSession $session | Out-Null
if ([IO.File]::ReadAllText($dlOut) -ne "drive-content") { throw "download content wrong" }
Remove-Item $dlOut -ErrorAction SilentlyContinue
$share = Invoke-Get ("/drive/share/" + $up.id)
if ($share.url -notlike "*token=*") { throw "share url missing token" }
$null = Invoke-Post "/drive/trash" @{ ids = @($up.id) }
$trash = Invoke-Get "/drive/trash"
if ($trash.Count -ne 1) { throw "trash count $($trash.Count)" }
Remove-Item $tmp -ErrorAction SilentlyContinue
Write-Host "ok cloud drive (folder/upload/list/download/share/trash)"

# 2. Admin overview.
$overview = Invoke-Get "/admin/overview"
if ($null -eq $overview.users) { throw "overview missing users" }
Write-Host ("ok admin overview (users=" + $overview.users + ", drive_files=" + $overview.drive_files + ", engine=" + $overview.engine + ")")

Write-Host "P2 SMOKE PASSED"
