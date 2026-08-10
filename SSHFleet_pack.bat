@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo   SSHFleet 构建脚本
echo ========================================
echo.

set "SCRIPT_DIR=%~dp0"
set "SRC=!SCRIPT_DIR!modules\SSHFleet_py"
set "BUILD=!SCRIPT_DIR!build"
set "RELEASE=!SCRIPT_DIR!release"

for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd_HH-mm-ss"') do set "TS=%%I"

set "DEST=!RELEASE!\SSHFleet_Py源码版_!TS!"

echo [1/6] 正在创建目录...
if not exist "!RELEASE!" mkdir "!RELEASE!"
mkdir "!DEST!"
echo        完成: !DEST!

echo.
echo [2/6] 正在复制入口文件...
if not exist "!SRC!\sshfleet.py" (
    echo [错误] 缺少入口文件
    pause
    exit /b 1
)
copy "!SRC!\sshfleet.py" "!DEST!\" >nul
echo        完成

echo.
echo [3/6] 正在复制 src 目录...
if not exist "!SRC!\src" (
    echo [错误] 缺少 src 目录
    pause
    exit /b 1
)
xcopy "!SRC!\src" "!DEST!\src" /E /I /Y /Q
echo        完成

echo.
echo [4/6] 正在复制 Go 二进制文件...
set "GO_SRC=!BUILD!"
set "GO_DEST=!DEST!\src\go"
if not exist "!GO_DEST!" mkdir "!GO_DEST!"
copy "!GO_SRC!\SSHFleet_Go.exe" "!GO_DEST!\" >nul
copy "!GO_SRC!\SSHFleet_Go" "!GO_DEST!\" >nul
echo        完成

echo.
echo [5/6] 正在复制 CHANGELOG.md...
if not exist "!SCRIPT_DIR!CHANGELOG.md" (
    echo [错误] 缺少 CHANGELOG.md
    pause
    exit /b 1
)
copy "!SCRIPT_DIR!CHANGELOG.md" "!DEST!\" >nul
echo        完成

echo.
echo [6/6] 正在复制 README.md...
if not exist "!SCRIPT_DIR!README.md" (
    echo [错误] 缺少 README.md
    pause
    exit /b 1
)
copy "!SCRIPT_DIR!README.md" "!DEST!\" >nul
echo        完成

echo.
echo ========================================
echo 构建完成!
echo 输出目录: !DEST!
echo ========================================

pause
