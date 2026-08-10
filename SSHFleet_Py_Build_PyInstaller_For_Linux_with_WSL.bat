@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo   SSHFleet PyInstaller 构建脚本 (WSL2 Linux)
echo ========================================
echo.

rem ============================================================
rem 配置 - 若你的发行版名称不同，请修改下面的 WSL_DISTRO
rem 使用: wsl --list --verbose   查看发行版名称
rem ============================================================
set "WSL_DISTRO=CentOS7"

set "SCRIPT_DIR=%~dp0"
set "BUILD_DIR=%SCRIPT_DIR%build"

rem ============================================================
rem 将 Windows 路径转换为 WSL 路径 (/mnt/d/...)
rem ============================================================
for /f "delims=" %%i in ('powershell -NoProfile -Command "$p='%SCRIPT_DIR:~0,-1%'; '/mnt/'+$p[0].ToString().ToLower()+($p.Substring(2) -replace '\\','/')"') do set "WSL_PROJECT=%%i"

rem Windows 源 (py 项目 + spec)
set "WSL_WIN_SRC=%WSL_PROJECT%/modules/SSHFleet_py"
set "WSL_WIN_SPEC=%WSL_PROJECT%/tools/Pyinstaller/SSHFleet.spec"

rem WSL 内部工作目录 (CentOS 自身文件系统，构建更快)
for /f "delims=" %%i in ('wsl -d %WSL_DISTRO% bash -c "echo $HOME" 2^>nul') do set "WSL_HOME=%%i"
if "%WSL_HOME%"=="" set "WSL_HOME=/root"
set "WSL_PROJ=%WSL_HOME%/sshfleet_build"
set "WSL_SRC=%WSL_PROJ%/modules/SSHFleet_py"
set "WSL_SPEC=%WSL_PROJ%/tools/Pyinstaller/SSHFleet.spec"
set "WSL_DIST=%WSL_SRC%/dist"
set "WSL_OUTPUT=%WSL_DIST%/SSHFleet"

rem ============================================================
rem [1/9] 检查 WSL
rem ============================================================
echo [1/9] 正在检查 WSL...
wsl -d %WSL_DISTRO% echo check >nul 2>&1
if errorlevel 1 (
    echo [错误] 无法访问 WSL 发行版: %WSL_DISTRO%
    echo         当前已安装的发行版:
    wsl --list --verbose
    echo.
    echo         请在本脚本顶部设置正确的 WSL_DISTRO
    pause
    exit /b 1
)
echo        WSL %WSL_DISTRO% 正常  (home: %WSL_HOME%)

rem ============================================================
rem [2/9] 检查 WSL 中的 Python
rem ============================================================
echo [2/9] 正在检查 WSL 中的 Python...
wsl -d %WSL_DISTRO% python3 --version >nul 2>&1
if errorlevel 1 (
    echo [错误] WSL 中未找到 Python3
    echo         安装命令: sudo yum install python3
    pause
    exit /b 1
)
for /f "tokens=2" %%i in ('wsl -d %WSL_DISTRO% python3 --version 2^>^&1') do set "PY_VER=%%i"
echo        Python: %PY_VER%

rem ============================================================
rem [3/9] 在 WSL 中检查 PyInstaller 与 rsync (缺失则报错退出，不自动安装)
rem ============================================================
echo [3/9] 正在检查 PyInstaller 与 rsync...
wsl -d %WSL_DISTRO% bash -c "command -v rsync" >nul 2>&1
if errorlevel 1 (
    echo [错误] WSL 中未找到 rsync，请先安装: sudo yum install rsync
    pause
    exit /b 1
)
wsl -d %WSL_DISTRO% bash -c 'python3 -c "import PyInstaller"' >nul 2>&1
if errorlevel 1 (
    echo [错误] WSL 中未找到 PyInstaller，请先安装: python3 -m pip install pyinstaller
    pause
    exit /b 1
)
for /f %%i in ('wsl -d %WSL_DISTRO% bash -c 'python3 -c "import PyInstaller; print(PyInstaller.__version__)"' 2^>nul') do set "PYI_VER=%%i"
echo        PyInstaller: %PYI_VER%

rem ============================================================
rem [4/9] 同步 (上传) 最新的 py 文件到 WSL CentOS
rem ============================================================
echo [4/9] 正在同步最新的 py 文件到 WSL CentOS...
echo        来源: %WSL_WIN_SRC%
echo        目标: %WSL_SRC%
wsl -d %WSL_DISTRO% bash -c "mkdir -p %WSL_SRC% %WSL_PROJ%/tools/Pyinstaller"
wsl -d %WSL_DISTRO% bash -c "rsync -a --delete --exclude='__pycache__' --exclude='*.pyc' --exclude='historys' --exclude='.git' '%WSL_WIN_SRC%/' '%WSL_SRC%/'" 2>&1
if not errorlevel 1 goto rsync1_ok
echo [错误] rsync 同步失败 - modules/SSHFleet_py
pause
exit /b 1
:rsync1_ok
wsl -d %WSL_DISTRO% bash -c "rsync -a --delete '%WSL_WIN_SPEC%' '%WSL_PROJ%/tools/Pyinstaller/SSHFleet.spec'" 2>&1
if not errorlevel 1 goto rsync2_ok
echo [错误] rsync 同步失败 - SSHFleet.spec
pause
exit /b 1
:rsync2_ok
echo        完成 (仅传输发生变化的文件)

rem ============================================================
rem [5/9] 在 WSL 中校验源文件
rem ============================================================
echo [5/9] 正在校验 WSL 中的源文件...
wsl -d %WSL_DISTRO% bash -c "test -f %WSL_SRC%/sshfleet.py"
if errorlevel 1 (
    echo [错误] 缺少 %WSL_SRC%/sshfleet.py
    pause
    exit /b 1
)
wsl -d %WSL_DISTRO% bash -c "test -f %WSL_SPEC%"
if errorlevel 1 (
    echo [错误] 缺少 %WSL_SPEC%
    pause
    exit /b 1
)
echo        完成

rem ============================================================
rem [6/9] 清理 WSL 中的旧构建产物
rem ============================================================
echo [6/9] 正在清理 WSL 中的旧构建产物...
wsl -d %WSL_DISTRO% bash -c "rm -rf %WSL_SRC%/dist %WSL_SRC%/build %WSL_SRC%/historys %WSL_SRC%/__pycache__ %WSL_SRC%/src/__pycache__ %WSL_SRC%/src/gotogo/__pycache__ %WSL_SRC%/src/transfer/__pycache__"
echo        完成

rem ============================================================
rem [7/9] 在 WSL 中使用 PyInstaller 构建
rem ============================================================
echo [7/9] 正在使用 PyInstaller 在 WSL 中构建...
wsl -d %WSL_DISTRO% bash -c "cd %WSL_SRC% && python3 -m PyInstaller --clean --noconfirm %WSL_SPEC%" 2>&1
if not errorlevel 1 goto build_ok
echo [错误] 构建失败
pause
exit /b 1
:build_ok
echo        完成

rem ============================================================
rem [8/9] 准备构建目录并将产物复制回 Windows
rem ============================================================
echo [8/9] 正在准备构建目录并复制产物...
if not exist "%BUILD_DIR%" mkdir "%BUILD_DIR%"
wsl -d %WSL_DISTRO% bash -c "test -f %WSL_OUTPUT%"
if errorlevel 1 (
    echo [错误] 未找到构建产物: %WSL_OUTPUT%
    pause
    exit /b 1
)
rem 通过 /mnt 直接将 Linux 二进制文件复制到 Windows 的 build 目录
wsl -d %WSL_DISTRO% bash -c "cp -f %WSL_OUTPUT% %WSL_PROJECT%/build/SSHFleet"
if errorlevel 1 (
    echo [错误] 复制产物到 build/ 失败
    pause
    exit /b 1
)
echo        已复制 SSHFleet (Linux 二进制文件) 到 build\

rem ============================================================
rem [9/9] 完成
rem ============================================================
echo.
echo ============================================================
echo   构建完成!
echo   输出: %BUILD_DIR%\SSHFleet
echo ============================================================
echo.
echo   说明: 这是基于 WSL2 CentOS 构建的 Linux ELF 二进制文件，
echo         用于部署到 Linux 服务器。
echo   如需 Windows 版本构建，请使用:
echo         SSHFleet_Py_build_PyInstaller_For_Windows.bat
echo.

pause
exit /b 0
