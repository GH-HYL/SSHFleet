@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo   SSHFleet PyInstaller 构建脚本 (Windows)
echo ========================================
echo.

set "SCRIPT_DIR=%~dp0"
set "SRC=!SCRIPT_DIR!modules\SSHFleet_py"
set "BUILD=!SCRIPT_DIR!build"
set "RELEASE=!SCRIPT_DIR!release"
set "PY_TOOLS=!SCRIPT_DIR!tools\Pyinstaller"

for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd_HH-mm-ss"') do set "TS=%%I"
set "DEST=!RELEASE!\SSHFleet_Py打包版_!TS!"

echo [1/8] 正在检查 Python...
python --version >nul 2>&1
if errorlevel 1 (
    echo [错误] 未找到 Python
    pause
    exit /b 1
)
for /f "tokens=2" %%i in ('python --version 2^>^&1') do set PYTHON_VERSION=%%i
echo        Python: %PYTHON_VERSION%

echo [2/8] 正在检查 PyInstaller...
cd /d "!PY_TOOLS!"
python -m PyInstaller --version >nul 2>&1
if errorlevel 1 (
    echo [错误] 未找到 PyInstaller，请先安装: pip install pyinstaller
    pause
    exit /b 1
)
for /f %%i in ('python -m PyInstaller --version 2^>^&1') do set PYINSTALLER_VERSION=%%i
echo        PyInstaller: %PYINSTALLER_VERSION%

echo [3/8] 正在检查源文件...
if not exist "!SRC!\sshfleet.py" (
    echo [错误] 缺少 !SRC!\sshfleet.py
    pause
    exit /b 1
)
if not exist "!PY_TOOLS!\SSHFleet.spec" (
    echo [错误] 缺少 SSHFleet.spec
    pause
    exit /b 1
)
echo        完成

echo [4/8] 正在清理旧的构建产物...
if exist "!PY_TOOLS!\build" rmdir /s /q "!PY_TOOLS!\build"
if exist "!PY_TOOLS!\dist" rmdir /s /q "!PY_TOOLS!\dist"
if exist "!SRC!\historys" rmdir /s /q "!SRC!\historys"
if exist "!SRC!\__pycache__" rmdir /s /q "!SRC!\__pycache__"
if exist "!SRC!\src\__pycache__" rmdir /s /q "!SRC!\src\__pycache__"
if exist "!SRC!\src\gotogo\__pycache__" rmdir /s /q "!SRC!\src\gotogo\__pycache__"
if exist "!SRC!\src\transfer\__pycache__" rmdir /s /q "!SRC!\src\transfer\__pycache__"
echo        完成

echo [5/8] 正在使用 PyInstaller 构建...
python -m PyInstaller --clean --noconfirm SSHFleet.spec
if errorlevel 1 (
    echo [错误] 构建失败
    cd /d "!SCRIPT_DIR!"
    pause
    exit /b 1
)
echo        完成

echo [6/8] 正在创建发布目录...
cd /d "!SCRIPT_DIR!"
if not exist "!RELEASE!" mkdir "!RELEASE!"
mkdir "!DEST!"
echo        完成: !DEST!

echo [7/8] 正在复制文件...
if not exist "!PY_TOOLS!\dist\SSHFleet.exe" (
    echo [错误] 未找到 SSHFleet.exe
    pause
    exit /b 1
)
xcopy /s /e /y /h "!PY_TOOLS!\dist\*" "!DEST!\" >nul
echo        已复制 exe

if not exist "!SCRIPT_DIR!README.md" (
    echo [错误] 缺少 README.md
    pause
    exit /b 1
)
copy "!SCRIPT_DIR!README.md" "!DEST!\" >nul
echo        已复制 README.md

if not exist "!SCRIPT_DIR!CHANGELOG.md" (
    echo [错误] 缺少 CHANGELOG.md
    pause
    exit /b 1
)
copy "!SCRIPT_DIR!CHANGELOG.md" "!DEST!\" >nul
echo        已复制 CHANGELOG.md

echo [8/8] 正在复制用户配置...
if not exist "!SRC!\src\config" (
    echo [警告] 没有配置目录，已跳过
) else (
    xcopy /s /e /y /q "!SRC!\src\config" "!DEST!\src\config\" >nul
    echo        已复制 src\config
)

if not exist "!BUILD!\SSHFleet_Go.exe" (
    echo [提示] 构建目录中未找到 Go 二进制文件，已跳过
) else (
    if not exist "!DEST!\src\go" mkdir "!DEST!\src\go"
    copy "!BUILD!\SSHFleet_Go.exe" "!DEST!\src\go\" >nul
    copy "!BUILD!\SSHFleet_Go" "!DEST!\src\go\" >nul 2>&1
    echo        已复制 src\go
)

if not exist "!BUILD!\SSHFleet" (
    echo [提示] 构建目录中未找到 Linux 打包文件 build\SSHFleet，已跳过
) else (
    copy "!BUILD!\SSHFleet" "!DEST!\" >nul
    echo        已复制 Linux 打包文件 SSHFleet
)

echo.
echo ========================================
echo   构建完成!
echo   输出目录: !DEST!
echo ========================================

pause
exit
