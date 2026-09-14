@echo off
rem SSHFleet 构建脚本（Windows，双击即可跑）
rem   编译 windows/amd64 + linux/amd64，产物落工作区根 build\
rem   带 release 参数：额外组装发布目录并打 zip（release\SSHFleet_<版本>_windows.zip）
rem   带任意参数运行都会跳过结尾暂停（供自动化/脚本调用）；双击（无参数）行为不变
rem 注意：本文件必须是 UTF-8 带 BOM + CRLF，否则 cmd 按 GBK 解析，中文注释会碎成乱行并当命令执行

rem 先记下原代码页再切 UTF-8：chcp 不受 setlocal 约束，从既有窗口调用时要还原回去
for /f "tokens=2 delims=:" %%c in ('chcp') do set "OLDCP=%%c"
set "OLDCP=%OLDCP: =%"
chcp 65001 >nul
setlocal

rem 前置检查一：go 必须在 PATH 里，否则后面只会报「构建失败」，误导为代码问题
where go >nul 2>nul || (echo [失败] 未找到 go，请先安装 Go 并加入 PATH & set FAILED=1)

rem 前置检查二：子模块目录必须先确认存在。cd 失败时脚本不会停，会继续在「当前目录恰好存在的
rem 某个 Go 包」上构建出产物却不报错（2026-09-14 实测过这个误建场景）
if not defined FAILED if not exist "%~dp0modules\SSHFleet_Go\" (echo [失败] 找不到子模块目录：%~dp0modules\SSHFleet_Go & set FAILED=1)
if defined FAILED goto :finish

cd /d "%~dp0modules\SSHFleet_Go"
set "OUT=%~dp0build"
if not exist "%OUT%" mkdir "%OUT%"

echo [1/2] 编译 windows/amd64 ...
set GOOS=windows
set GOARCH=amd64
go build -o "%OUT%\SSHFleet.exe" . || (echo [失败] windows/amd64 构建失败 & set FAILED=1)
if defined FAILED goto :finish
echo [完成] %OUT%\SSHFleet.exe

echo [2/2] 交叉编译 linux/amd64 ...
set GOOS=linux
set GOARCH=amd64
go build -o "%OUT%\SSHFleet" . || (echo [失败] linux/amd64 构建失败 & set FAILED=1)
if defined FAILED goto :finish
echo [完成] %OUT%\SSHFleet

echo.
echo 构建完成!
dir "%OUT%" /T:W

rem ---- release 模式：组发布目录 + 打 zip -----------------------------------
rem Linux 的包由 build-linux.sh release 负责（tar.gz）：各平台的包在各自平台上打，
rem 权限与容器格式都最可靠（Windows 的 bsdtar 打不出带执行权限的 tar.gz）
if /i not "%~1"=="release" goto :finish

rem zip 用 Windows 10 1803+ 自带的 tar.exe（bsdtar，按扩展名自动选格式）
where tar >nul 2>nul || (echo [失败] 未找到 tar（Windows 10 1803+ 自带），无法生成 zip & set FAILED=1)
if defined FAILED goto :finish

rem 版本号取自 CHANGELOG.md 顶部第一个正式版本号（形如 ## [5.0.0]）；取不到用当天日期
set "VER="
for /f "tokens=2 delims=[]" %%v in ('findstr /r "^## \[[0-9]" "%~dp0CHANGELOG.md"') do if not defined VER set "VER=%%v"
if not defined VER set "VER=%DATE:~0,10%"
set "VER=%VER:/=-%"

set "REL=%~dp0release"
set "NAME=SSHFleet_%VER%_windows"
set "PKGDIR=%REL%\%NAME%"
echo.
echo [发布] 组装 release\%NAME% ...
if exist "%PKGDIR%" rmdir /s /q "%PKGDIR%"
mkdir "%PKGDIR%\config"
copy /y "%OUT%\SSHFleet.exe" "%PKGDIR%\" >nul
copy /y "%~dp0README.md" "%PKGDIR%\" >nul
copy /y "%~dp0CHANGELOG.md" "%PKGDIR%\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\SSHFleet.conf" "%PKGDIR%\config\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\dangerous_keywords.toml" "%PKGDIR%\config\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\error_keywords.toml" "%PKGDIR%\config\" >nul

if exist "%REL%\%NAME%.zip" del /q "%REL%\%NAME%.zip"
tar -a -cf "%REL%\%NAME%.zip" -C "%REL%" "%NAME%" || (echo [失败] 生成 zip 失败 & set FAILED=1)
if defined FAILED goto :finish

echo [发布] 完成：release\%NAME%.zip
tar -tf "%REL%\%NAME%.zip"

:finish
if defined OLDCP chcp %OLDCP% >nul
if "%~1"=="" pause
if defined FAILED exit /b 1
exit /b 0
