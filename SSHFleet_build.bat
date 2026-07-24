@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo   SSHFleet Build Script
echo ========================================
echo.

set "SCRIPT_DIR=%~dp0"
set "SRC=!SCRIPT_DIR!modules\SSHFleet_py"
set "BUILD=!SCRIPT_DIR!build"
set "RELEASE=!SCRIPT_DIR!release"

for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd_HH-mm-ss"') do set "TS=%%I"

set "DEST=!RELEASE!\!TS!"

echo [1/6] Creating directory...
if not exist "!RELEASE!" mkdir "!RELEASE!"
mkdir "!DEST!"
echo        OK: !DEST!

echo.
echo [2/6] Copying entry file...
if not exist "!SRC!\sshfleet.py" (
    echo [ERROR] Missing entry file
    pause
    exit /b 1
)
copy "!SRC!\sshfleet.py" "!DEST!\" >nul
echo        OK

echo.
echo [3/6] Copying src directory...
if not exist "!SRC!\src" (
    echo [ERROR] Missing src directory
    pause
    exit /b 1
)
xcopy "!SRC!\src" "!DEST!\src" /E /I /Y /Q
echo        OK

echo.
echo [4/6] Copying Go binaries...
set "GO_SRC=!BUILD!"
set "GO_DEST=!DEST!\src\go"
if not exist "!GO_DEST!" mkdir "!GO_DEST!"
copy "!GO_SRC!\SSHFleet_Go.exe" "!GO_DEST!\" >nul
copy "!GO_SRC!\SSHFleet_Go" "!GO_DEST!\" >nul
echo        OK

echo.
echo [5/6] Copying CHANGELOG.md...
if not exist "!SCRIPT_DIR!CHANGELOG.md" (
    echo [ERROR] Missing CHANGELOG.md
    pause
    exit /b 1
)
copy "!SCRIPT_DIR!CHANGELOG.md" "!DEST!\" >nul
echo        OK

echo.
echo [6/6] Copying README.md...
if not exist "!SRC!\README.md" (
    echo [ERROR] Missing README.md
    pause
    exit /b 1
)
copy "!SRC!\README.md" "!DEST!\" >nul
echo        OK

echo.
echo ========================================
echo Build completed!
echo Output: !DEST!
echo ========================================

pause
