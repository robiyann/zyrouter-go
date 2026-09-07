$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

if (-not $env:PORT) { $env:PORT = "20128" }
if (-not $env:HOST) { $env:HOST = "127.0.0.1" }
if (-not $env:FRONTEND_DIR) { $env:FRONTEND_DIR = ".\frontend" }

Write-Host "===================================================" -ForegroundColor Cyan
Write-Host "  Zyrouter AI Gateway & Admin Dashboard" -ForegroundColor Cyan
Write-Host "  Listening on http://$($env:HOST):$($env:PORT)" -ForegroundColor Green
Write-Host "===================================================" -ForegroundColor Cyan

if (Test-Path ".\zyrouter.exe") {
    & .\zyrouter.exe
} elseif (Test-Path ".\backend\zyrouter.exe") {
    & .\backend\zyrouter.exe
} else {
    Write-Host "Error: zyrouter.exe not found!" -ForegroundColor Red
}
