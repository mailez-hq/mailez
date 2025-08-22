# Generates a self-signed certificate for local TLS testing.
# Usage:  .\generate-certs.ps1 -Hostname "mail.example.com"
# Output: deploy/certs/cert.pem + deploy/certs/key.pem (gitignored)
param(
    [string]$Hostname = "mail.example.com"
)

$ErrorActionPreference = "Stop"

# openssl is not on PATH by default on Windows; fall back to the Git install.
$openssl = Get-Command openssl -ErrorAction SilentlyContinue
if (-not $openssl) {
    $gitOpenSSL = "C:\Program Files\Git\usr\bin\openssl.exe"
    if (Test-Path $gitOpenSSL) {
        $openssl = $gitOpenSSL
    } else {
        throw "openssl not found. Install Git for Windows or add openssl to PATH."
    }
} else {
    $openssl = $openssl.Source
}

$certsDir = Join-Path (Split-Path $PSScriptRoot -Parent) "certs"
New-Item -ItemType Directory -Force -Path $certsDir | Out-Null

$cert = Join-Path $certsDir "cert.pem"
$key = Join-Path $certsDir "key.pem"

& $openssl req -x509 -newkey rsa:2048 `
    -keyout $key -out $cert -days 365 -nodes `
    -subj "/CN=$Hostname" `
    -addext "subjectAltName=DNS:$Hostname,DNS:localhost,IP:127.0.0.1"

Write-Host "Wrote $cert and $key (self-signed, dev only)."
