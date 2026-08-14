@echo off
chcp 65001 >nul
setlocal enabledelayedexpansion

echo ========================================
echo   SSHFleet 一键构建 (整合版)
echo   顺序: 删release, Go, 清缓存, Pack, WSL(Linux), Windows, 清缓存, 最后分目录打包 zip
echo ========================================
echo.

REM 项目根目录 = 本脚本所在目录 (不写死盘符)
set "SCRIPT_DIR=%~dp0"

REM ============================================================
REM [0] 开头先删除 release 目录
REM ============================================================
echo [准备] 正在删除旧的 release 目录...
if exist "%SCRIPT_DIR%release" (
    rmdir /s /q "%SCRIPT_DIR%release"
    echo        已删除 release\
) else (
    echo        release\ 不存在，跳过
)
echo.

REM ============================================================
REM [1/4] SSHFleet Go 构建 (Windows + Linux)
REM     原 SSHFleet_Go_build.bat 内容 (路径改为基于 SCRIPT_DIR)
REM ============================================================
echo [1/4] SSHFleet Go 构建 (Windows + Linux)...
set "DEPLOY_DIR=%SCRIPT_DIR%build"
if not exist "%DEPLOY_DIR%" mkdir "%DEPLOY_DIR%"

pushd "%SCRIPT_DIR%modules\SSHFleet_go"

echo        [1/2] 正在构建 Windows...
set GOOS=windows
set GOARCH=amd64
go build -o "%DEPLOY_DIR%\SSHFleet_Go.exe" .
if errorlevel 1 (
    echo [失败] Windows 构建失败
    popd
    goto :fail
)
echo        [完成] %DEPLOY_DIR%\SSHFleet_Go.exe

echo        [2/2] 正在构建 Linux...
set GOOS=linux
set GOARCH=amd64
go build -o "%DEPLOY_DIR%\SSHFleet_Go" .
if errorlevel 1 (
    echo [失败] Linux 构建失败
    popd
    goto :fail
)
echo        [完成] %DEPLOY_DIR%\SSHFleet_Go

popd
echo [完成] Go 构建完成
echo.

REM ============================================================
REM [2/4] 打包 Python 源码版
REM     原 SSHFleet_Pack.bat 内容
REM ============================================================
echo [清理 1/2] Pack 之前清理 Python 运行缓存...
call :clean_pycache
echo.
echo [2/4] 打包 Python 源码版...
set "SRC=%SCRIPT_DIR%modules\SSHFleet_py"
set "BUILD=%SCRIPT_DIR%build"
set "RELEASE=%SCRIPT_DIR%release"
for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd_HH-mm-ss"') do set "TS=%%I"
set "DEST=%RELEASE%\SSHFleet_Py源码版_%TS%"

if not exist "%RELEASE%" mkdir "%RELEASE%"
mkdir "%DEST%"
echo        目标: %DEST%

if not exist "%SRC%\sshfleet.py" ( echo [错误] 缺少入口文件 & goto :fail )
copy "%SRC%\sshfleet.py" "%DEST%\" >nul

if not exist "%SRC%\src" ( echo [错误] 缺少 src 目录 & goto :fail )
xcopy "%SRC%\src" "%DEST%\src" /E /I /Y /Q

set "GO_DEST=%DEST%\src\go"
if not exist "%GO_DEST%" mkdir "%GO_DEST%"
copy "%BUILD%\SSHFleet_Go.exe" "%GO_DEST%\" >nul
copy "%BUILD%\SSHFleet_Go" "%GO_DEST%\" >nul

if not exist "%SCRIPT_DIR%CHANGELOG.md" ( echo [错误] 缺少 CHANGELOG.md & goto :fail )
copy "%SCRIPT_DIR%CHANGELOG.md" "%DEST%\" >nul
if not exist "%SCRIPT_DIR%README.md" ( echo [错误] 缺少 README.md & goto :fail )
copy "%SCRIPT_DIR%README.md" "%DEST%\" >nul

echo [完成] 源码版打包完成: %DEST%
echo.

REM ============================================================
REM [3/4] WSL2 Linux PyInstaller 构建
REM     原 SSHFleet_Py_Build_PyInstaller_For_Linux_with_WSL.bat 内容
REM ============================================================
echo [3/4] WSL2 Linux PyInstaller 构建...
set "WSL_DISTRO=CentOS7"
set "BUILD_DIR=%SCRIPT_DIR%build"

REM 将 Windows 路径转换为 WSL 路径 (/mnt/d/...)
for /f "delims=" %%i in ('powershell -NoProfile -Command "$p='%SCRIPT_DIR:~0,-1%'; '/mnt/'+$p[0].ToString().ToLower()+($p.Substring(2) -replace '\\','/')"') do set "WSL_PROJECT=%%i"

REM Windows 源 (py 项目 + spec)
set "WSL_WIN_SRC=%WSL_PROJECT%/modules/SSHFleet_py"
set "WSL_WIN_SPEC=%WSL_PROJECT%/tools/Pyinstaller/SSHFleet.spec"

REM WSL 内部工作目录
for /f "delims=" %%i in ('wsl -d %WSL_DISTRO% bash -c "echo $HOME" 2^>nul') do set "WSL_HOME=%%i"
if "%WSL_HOME%"=="" set "WSL_HOME=/root"
set "WSL_PROJ=%WSL_HOME%/sshfleet_build"
set "WSL_SRC=%WSL_PROJ%/modules/SSHFleet_py"
set "WSL_SPEC=%WSL_PROJ%/tools/Pyinstaller/SSHFleet.spec"
set "WSL_DIST=%WSL_SRC%/dist"
set "WSL_OUTPUT=%WSL_DIST%/SSHFleet"

echo        [1/9] 正在检查 WSL...
wsl -d %WSL_DISTRO% echo check >nul 2>&1
if errorlevel 1 (
    echo [错误] 无法访问 WSL 发行版: %WSL_DISTRO%
    wsl --list --verbose
    goto :fail
)
echo        [2/9] 正在检查 WSL 中的 Python...
wsl -d %WSL_DISTRO% python3 --version >nul 2>&1
if errorlevel 1 ( echo [错误] WSL 中未找到 Python3 & goto :fail )
echo        [3/9] 正在检查 PyInstaller 与 rsync...
wsl -d %WSL_DISTRO% bash -c "command -v rsync" >nul 2>&1
if errorlevel 1 ( echo [错误] WSL 中未找到 rsync & goto :fail )
wsl -d %WSL_DISTRO% bash -c 'python3 -c "import PyInstaller"' >nul 2>&1
if errorlevel 1 ( echo [错误] WSL 中未找到 PyInstaller & goto :fail )
echo        [4/9] 正在同步最新的 py 文件到 WSL...
wsl -d %WSL_DISTRO% bash -c "mkdir -p %WSL_SRC% %WSL_PROJ%/tools/Pyinstaller"
wsl -d %WSL_DISTRO% bash -c "rsync -a --delete --exclude='__pycache__' --exclude='*.pyc' --exclude='historys' --exclude='.git' '%WSL_WIN_SRC%/' '%WSL_SRC%/'" 2>&1
if errorlevel 1 ( echo [错误] rsync 同步失败 - modules/SSHFleet_py & goto :fail )
wsl -d %WSL_DISTRO% bash -c "rsync -a --delete '%WSL_WIN_SPEC%' '%WSL_PROJ%/tools/Pyinstaller/SSHFleet.spec'" 2>&1
if errorlevel 1 ( echo [错误] rsync 同步失败 - SSHFleet.spec & goto :fail )
echo        [5/9] 正在校验 WSL 中的源文件...
wsl -d %WSL_DISTRO% bash -c "test -f %WSL_SRC%/sshfleet.py"
if errorlevel 1 ( echo [错误] 缺少 %WSL_SRC%/sshfleet.py & goto :fail )
wsl -d %WSL_DISTRO% bash -c "test -f %WSL_SPEC%"
if errorlevel 1 ( echo [错误] 缺少 %WSL_SPEC% & goto :fail )
echo        [6/9] 正在清理 WSL 中的旧构建产物...
wsl -d %WSL_DISTRO% bash -c "rm -rf %WSL_SRC%/dist %WSL_SRC%/build %WSL_SRC%/historys %WSL_SRC%/__pycache__ %WSL_SRC%/src/__pycache__ %WSL_SRC%/src/gotogo/__pycache__ %WSL_SRC%/src/transfer/__pycache__"
echo        [7/9] 正在使用 PyInstaller 在 WSL 中构建...
wsl -d %WSL_DISTRO% bash -c "cd %WSL_SRC% && python3 -m PyInstaller --clean --noconfirm %WSL_SPEC%" 2>&1
if errorlevel 1 ( echo [错误] 构建失败 & goto :fail )
echo        [8/9] 正在准备构建目录并复制产物...
if not exist "%BUILD_DIR%" mkdir "%BUILD_DIR%"
wsl -d %WSL_DISTRO% bash -c "test -f %WSL_OUTPUT%"
if errorlevel 1 ( echo [错误] 未找到构建产物: %WSL_OUTPUT% & goto :fail )
wsl -d %WSL_DISTRO% bash -c "cp -f %WSL_OUTPUT% %WSL_PROJECT%/build/SSHFleet"
if errorlevel 1 ( echo [错误] 复制产物到 build\ 失败 & goto :fail )
echo        已复制 SSHFleet (Linux 二进制文件) 到 build\
echo [完成] WSL Linux 构建完成
echo.

REM ============================================================
REM [4/4] Windows PyInstaller 构建
REM     原 SSHFleet_Py_Build_PyInstaller_For_Windows.bat 内容
REM ============================================================
echo [4/4] Windows PyInstaller 构建...
set "SRC=%SCRIPT_DIR%modules\SSHFleet_py"
set "BUILD=%SCRIPT_DIR%build"
set "RELEASE=%SCRIPT_DIR%release"
set "PY_TOOLS=%SCRIPT_DIR%tools\Pyinstaller"
for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd_HH-mm-ss"') do set "TS=%%I"
set "DEST=%RELEASE%\SSHFleet_Py打包版_%TS%"

echo        [1/8] 正在检查 Python...
python --version >nul 2>&1
if errorlevel 1 ( echo [错误] 未找到 Python & goto :fail )
echo        [2/8] 正在检查 PyInstaller...
pushd "%PY_TOOLS%"
python -m PyInstaller --version >nul 2>&1
if errorlevel 1 ( popd & echo [错误] 未找到 PyInstaller，请先安装: pip install pyinstaller & goto :fail )
echo        [3/8] 正在检查源文件...
if not exist "%SRC%\sshfleet.py" ( popd & echo [错误] 缺少 %SRC%\sshfleet.py & goto :fail )
if not exist "%PY_TOOLS%\SSHFleet.spec" ( popd & echo [错误] 缺少 SSHFleet.spec & goto :fail )
echo        [4/8] 正在清理旧的构建产物...
if exist "%PY_TOOLS%\build" rmdir /s /q "%PY_TOOLS%\build"
if exist "%PY_TOOLS%\dist" rmdir /s /q "%PY_TOOLS%\dist"
if exist "%SRC%\historys" rmdir /s /q "%SRC%\historys"
if exist "%SRC%\__pycache__" rmdir /s /q "%SRC%\__pycache__"
if exist "%SRC%\src\__pycache__" rmdir /s /q "%SRC%\src\__pycache__"
if exist "%SRC%\src\gotogo\__pycache__" rmdir /s /q "%SRC%\src\gotogo\__pycache__"
if exist "%SRC%\src\transfer\__pycache__" rmdir /s /q "%SRC%\src\transfer\__pycache__"
echo        [5/8] 正在使用 PyInstaller 构建...
python -m PyInstaller --clean --noconfirm SSHFleet.spec
if errorlevel 1 ( popd & echo [错误] 构建失败 & goto :fail )
popd
echo        [6/8] 正在创建发布目录...
if not exist "%RELEASE%" mkdir "%RELEASE%"
mkdir "%DEST%"
echo        [7/8] 正在复制文件...
if not exist "%PY_TOOLS%\dist\SSHFleet.exe" ( echo [错误] 未找到 SSHFleet.exe & goto :fail )
xcopy /s /e /y /h "%PY_TOOLS%\dist\*" "%DEST%\" >nul
if not exist "%SCRIPT_DIR%README.md" ( echo [错误] 缺少 README.md & goto :fail )
copy "%SCRIPT_DIR%README.md" "%DEST%\" >nul
if not exist "%SCRIPT_DIR%CHANGELOG.md" ( echo [错误] 缺少 CHANGELOG.md & goto :fail )
copy "%SCRIPT_DIR%CHANGELOG.md" "%DEST%\" >nul
echo        [8/8] 正在复制用户配置与 Go 二进制...
if exist "%SRC%\src\config" (
    xcopy /s /e /y /q "%SRC%\src\config" "%DEST%\src\config\" >nul
)
if exist "%BUILD%\SSHFleet_Go.exe" (
    if not exist "%DEST%\src\go" mkdir "%DEST%\src\go"
    copy "%BUILD%\SSHFleet_Go.exe" "%DEST%\src\go\" >nul
    copy "%BUILD%\SSHFleet_Go" "%DEST%\src\go\" >nul 2>&1
)
if exist "%BUILD%\SSHFleet" (
    copy "%BUILD%\SSHFleet" "%DEST%\" >nul
)
echo [完成] Windows 打包版完成: %DEST%
echo.
echo [清理 2/2] Windows 构建之后清理 Python 运行缓存...
call :clean_pycache
echo.

REM ============================================================
REM [5] 将 release 下生成的每个目录分别打包为同名 zip
REM ============================================================
echo [5] 正在将 release 下各目录分别打包为 zip...
set "ZIP_COUNT=0"
pushd "%RELEASE%"
for /d %%D in (*) do (
    echo        正在打包: %%~nxD
    "%SystemRoot%\System32\tar.exe" -a -cf "%%~nxD.zip" "%%~nxD"
    if errorlevel 1 ( popd & echo [错误] 打包失败: %%~nxD & goto :fail )
    set /a ZIP_COUNT+=1
)
popd
echo        已打包 %ZIP_COUNT% 个目录
echo.

echo ========================================
echo   全部完成!
echo   发布目录: %RELEASE%
echo ========================================
dir "%RELEASE%" /T:W
goto :end

:fail
echo.
echo ========================================
echo   [构建失败] 已中止，请查看上方错误信息。
echo ========================================

:end
REM 结尾暂停，方便查看结果；若需完全无人值守，删除下面这行或设置环境变量 SSHFLEET_NO_PAUSE=1
if not defined SSHFLEET_NO_PAUSE pause
exit /b 0

REM ============================================================
REM 子过程: 清理 Python 运行缓存 (由 Clean_Py_Cache.bat 整合)
REM 删除 %SCRIPT_DIR%modules 下的 __pycache__/.pytest_cache 文件夹
REM 与 *.pyc/*.pyo 编译缓存文件
REM ============================================================
:clean_pycache
set "TARGET=%SCRIPT_DIR%modules"
if not exist "%TARGET%" (
    echo [错误] 未找到目录: %TARGET%
    goto :eof
)
set /a DEL_DIR=0
set /a DEL_FILE=0
for /f "delims=" %%d in ('dir /b /s /ad "%TARGET%\__pycache__" 2^>nul') do (
    rd /s /q "%%d" 2>nul
    if exist "%%d" ( echo   [失败] 无法删除文件夹: %%d ) else ( set /a DEL_DIR+=1 )
)
for /f "delims=" %%d in ('dir /b /s /ad "%TARGET%\.pytest_cache" 2^>nul') do (
    rd /s /q "%%d" 2>nul
    if exist "%%d" ( echo   [失败] 无法删除文件夹: %%d ) else ( set /a DEL_DIR+=1 )
)
for /f "delims=" %%f in ('dir /b /s /a-d "%TARGET%\*.pyc" 2^>nul') do (
    del /f /q "%%f" 2>nul
    if exist "%%f" ( echo   [失败] 无法删除文件: %%f ) else ( set /a DEL_FILE+=1 )
)
for /f "delims=" %%f in ('dir /b /s /a-d "%TARGET%\*.pyo" 2^>nul') do (
    del /f /q "%%f" 2>nul
    if exist "%%f" ( echo   [失败] 无法删除文件: %%f ) else ( set /a DEL_FILE+=1 )
)
echo        清理完成: 删除文件夹 %DEL_DIR% 个, 删除文件 %DEL_FILE% 个
goto :eof
