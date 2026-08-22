#!/usr/bin/env pwsh
# Build the mailez mail-stack images from the vendored component sources
# (deploy/vendor/mailstack) and tag them as mailez/<component>:local.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File deploy/scripts/build-images.ps1
#
# After building, switch the compose files to the local images:
#   $env:MAILEZ_IMAGE_PREFIX='mailez'; $env:MAILEZ_IMAGE_TAG='local'
#   docker compose -f docker-compose.dev.yml up -d
#
# Build order matters: base first, then the components that FROM base.

$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$vendor = Join-Path $root 'deploy\vendor\mailstack'
$docker = $env:DOCKER_CMD
if (-not $docker) {
  $candidates = @(
    'docker',
    'C:\Users\admin\AppData\Local\Programs\DockerDesktop\resources\bin\docker.exe',
    'C:\Program Files\Docker\Docker\resources\bin\docker.exe'
  )
  foreach ($c in $candidates) {
    if (Get-Command $c -ErrorAction SilentlyContinue) { $docker = $c; break }
    if (Test-Path -LiteralPath $c) { $docker = $c; break }
  }
}
if (-not $docker) { throw 'docker CLI not found; set $env:DOCKER_CMD to its path' }

& $docker version > $null
if ($LASTEXITCODE -ne 0) { throw 'docker daemon is not running' }

function Build-Image([string]$name, [string]$dir) {
  Write-Host "==> building mailez/$name :local from $dir"
  & $docker build --build-arg VERSION=local -t "mailez/$name`:local" $dir
  if ($LASTEXITCODE -ne 0) { throw "build failed: mailez/$name" }
}

Build-Image 'base' (Join-Path $vendor 'base')
foreach ($component in @('nginx','dovecot','postfix','rspamd','oletools','unbound')) {
  Build-Image $component (Join-Path $vendor $component)
}

Write-Host ''
Write-Host 'All images built. The compose files reference mailez/*:local by'
Write-Host 'default, so docker compose up now runs fully self-hosted images.'
