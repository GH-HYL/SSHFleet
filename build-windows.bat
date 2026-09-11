@echo off
rem SSHFleet 构建脚本（Windows，双击即可跑）
rem 编译 windows/amd64 + linux/amd64，产物落工作区根 build\
chcp 65001 >nul
setlocal
cd /d "%~dp0modules\SSHFleet_Go"
set "OUT=%~dp0build"
if not exist "%OUT%" mkdir "%OUT%"

echo [1/2] 编译 windows/amd64 ...
set GOOS=windows
set GOARCH=amd64
go build -o "%OUT%\SSHFleet.exe" .
if %errorlevel% neq 0 (
    echo [失败] windows/amd64 构建失败
    pause
    exit /b 1
)
echo [完成] %OUT%\SSHFleet.exe

echo [2/2] 交叉编译 linux/amd64 ...
set GOOS=linux
set GOARCH=amd64
go build -o "%OUT%\SSHFleet" .
if %errorlevel% neq 0 (
    echo [失败] linux/amd64 构建失败
    pause
    exit /b 1
)
echo [完成] %OUT%\SSHFleet

echo.
echo 构建完成!
dir "%OUT%" /T:W
pause
