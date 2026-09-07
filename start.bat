@echo off
setlocal
cd /d "%~dp0"

if "%PORT%"=="" set PORT=20128
if "%HOST%"=="" set HOST=127.0.0.1
if "%FRONTEND_DIR%"=="" set FRONTEND_DIR=.\frontend

echo ===================================================
echo   Zyrouter AI Gateway ^& Admin Dashboard
echo   Starting on http://%HOST%:%PORT%
echo ===================================================

if exist "zyrouter.exe" (
    .\zyrouter.exe
) else if exist "backend\zyrouter.exe" (
    .\backend\zyrouter.exe
) else (
    echo Error: zyrouter.exe not found!
    pause
)
