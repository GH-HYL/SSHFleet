@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo        SSHFleet PyInstaller 打包脚本
echo ========================================
echo.

:: 获取脚本所在目录
set "SCRIPT_DIR=%~dp0"
cd /d "%SCRIPT_DIR%"

:: 检查 Python 环境
echo [1/6] 检查 Python 环境...
python --version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] 未找到 Python，请确保已安装 Python 并添加到 PATH
    pause
    exit /b 1
)
for /f "tokens=2" %%i in ('python --version 2^>^&1') do set PYTHON_VERSION=%%i
echo        Python 版本: %PYTHON_VERSION%

:: 检查 PyInstaller
echo [2/6] 检查 PyInstaller...
pyinstaller --version >nul 2>&1
if errorlevel 1 (
    echo [INFO] PyInstaller 未安装，正在安装...
    pip install pyinstaller
    if errorlevel 1 (
        echo [ERROR] PyInstaller 安装失败
        pause
        exit /b 1
    )
)
for /f %%i in ('pyinstaller --version 2^>^&1') do set PYINSTALLER_VERSION=%%i
echo        PyInstaller 版本: %PYINSTALLER_VERSION%

:: 清理旧的构建文件
echo [3/6] 清理旧的构建文件...
if exist "build" (
    echo        删除 build 目录...
    rmdir /s /q "build"
)
if exist "dist" (
    echo        删除 dist 目录...
    rmdir /s /q "dist"
)

:: 删除 historys 目录（运行时生成的日志目录，打包时不需要）
if exist "historys" (
    echo        删除 historys 目录（运行时日志）...
    rmdir /s /q "historys"
)
if exist "__pycache__" (
    echo        删除 __pycache__ 目录...
    rmdir /s /q "__pycache__"
)
if exist "src\__pycache__" (
    echo        删除 src\__pycache__ 目录...
    rmdir /s /q "src\__pycache__"
)
if exist "src\gotogo\__pycache__" (
    echo        删除 src\gotogo\__pycache__ 目录...
    rmdir /s /q "src\gotogo\__pycache__"
)
if exist "src\transfer\__pycache__" (
    echo        删除 src\transfer\__pycache__ 目录...
    rmdir /s /q "src\transfer\__pycache__"
)
echo        清理完成

:: 检查必要的数据文件
echo [4/6] 检查数据文件...
set "MISSING_FILES=0"
if not exist "src\config\SSHFleet.yaml" (
    echo        [ERROR] 缺少 src\config\SSHFleet.yaml
    set "MISSING_FILES=1"
)
if not exist "src\config\dangerous_keywords.json" (
    echo        [ERROR] 缺少 src\config\dangerous_keywords.json
    set "MISSING_FILES=1"
)
if not exist "src\config\error_keywords.json" (
    echo        [ERROR] 缺少 src\config\error_keywords.json
    set "MISSING_FILES=1"
)
if not exist "src\go\SSHFleet_Go.exe" (
    echo        [WARNING] 缺少 src\go\SSHFleet_Go.exe（Go 可执行文件）
    echo                  打包后需要手动复制到输出目录
)
if %MISSING_FILES%==1 (
    echo        [ERROR] 数据文件缺失，无法继续打包
    pause
    exit /b 1
)
echo        数据文件检查通过

:: 执行 PyInstaller 打包
echo [5/6] 执行 PyInstaller 打包...
echo        使用 sshfleet.spec 配置文件...
pyinstaller --clean --noconfirm sshfleet.spec
if errorlevel 1 (
    echo        [ERROR] PyInstaller 打包失败
    pause
    exit /b 1
)
echo        打包完成

:: 复制 Go 可执行文件到输出目录
echo [6/6] 复制 Go 可执行文件...
if exist "src\go\SSHFleet_Go.exe" (
    if not exist "dist\SSHFleet\src\go" (
        mkdir "dist\SSHFleet\src\go"
    )
    copy "src\go\SSHFleet_Go.exe" "dist\SSHFleet\src\go\" >nul
    echo        已复制 SSHFleet_Go.exe
)
if exist "src\go\SSHFleet_Go" (
    if not exist "dist\SSHFleet\src\go" (
        mkdir "dist\SSHFleet\src\go"
    )
    copy "src\go\SSHFleet_Go" "dist\SSHFleet\src\go\" >nul
    echo        已复制 SSHFleet_Go
)

:: 显示打包结果
echo.
echo ========================================
echo           打包完成！
echo ========================================
echo.
echo 输出目录: dist\SSHFleet\
echo.
echo 目录结构:
echo   dist\SSHFleet\
echo   ├── SSHFleet.exe        # 主程序
echo   └── src\
echo       ├── config\        # 配置文件（已打包）
echo       │   ├── SSHFleet.yaml
echo       │   ├── dangerous_keywords.json
echo       │   └── error_keywords.json
echo       └── go\            # Go 可执行文件（已复制）
echo           ├── SSHFleet_Go.exe
echo           └── SSHFleet_Go
echo.
echo 注意: 运行时需要在 SSHFleet.exe 所在目录
echo ========================================
echo.

:: 询问是否打开输出目录
set /p "OPEN_DIR=是否打开输出目录？(Y/N): "
if /i "%OPEN_DIR%"=="Y" (
    explorer "dist\SSHFleet"
)

pause
