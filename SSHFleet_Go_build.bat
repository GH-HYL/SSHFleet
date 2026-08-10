@echo off
chcp 65001 >nul
echo SSHFleet_Go 构建脚本
echo ====================

set "DEPLOY_DIR=D:\Desktop\Code\SSHFleet\build"

cd /d modules\SSHFleet_go

if not exist "%DEPLOY_DIR%" mkdir "%DEPLOY_DIR%"

echo.
echo [1/2] 正在构建 Windows...
set GOOS=windows
set GOARCH=amd64
go build -o "%DEPLOY_DIR%\SSHFleet_Go.exe" .
if %errorlevel% neq 0 (
    echo [失败] Windows 构建失败
    pause
    exit /b 1
)
echo [完成] Windows: %DEPLOY_DIR%\SSHFleet_Go.exe

echo.
echo [2/2] 正在构建 Linux...
set GOOS=linux
set GOARCH=amd64
go build -o "%DEPLOY_DIR%\SSHFleet_Go" .
if %errorlevel% neq 0 (
    echo [失败] Linux 构建失败
    pause
    exit /b 1
)
echo [完成] Linux: %DEPLOY_DIR%\SSHFleet_Go

echo.
echo ====================
echo 构建完成!
echo.
dir "%DEPLOY_DIR%" /T:W

pause
