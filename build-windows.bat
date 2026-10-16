@echo off
rem SSHFleet 构建脚本（Windows，双击即可跑）
rem   双击直接走完全部流程：编译 windows/amd64 + linux/amd64（产物落工作区根 build\）
rem   → 一次出齐两个平台的发布产物：release\SSHFleet_<时间戳>_windows(.zip) 与
rem     release\SSHFleet_<时间戳>_linux(.tar.gz)，打包前先清空整个 release\
rem   带任意参数运行会跳过结尾暂停（供自动化/脚本调用）；双击（无参数）行为不变
rem 出厂配置按平台取：modules\SSHFleet_Go\config\ 下的 SSHFleet-for-windows.conf 与
rem   SSHFleet-for-linux.conf，各自复制成包内的 config\SSHFleet.conf。工作区里那份
rem   config\SSHFleet.conf 是作者本机在用的配置（内网端口、账号、凭据路径），不随包发布
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

rem 前置检查三：随包要拷的东西先确认在位。copy 匹配不到时只打印一行「找不到文件」，退出码不
rem 参与判定，包里会静默缺手册或缺出厂配置，而整轮跑下来照样报「全部完成」
if not defined FAILED if not exist "%~dp0docs\" (echo [失败] 找不到手册目录：%~dp0docs & set FAILED=1)
if not defined FAILED if not exist "%~dp0docs\?-*.md" (echo [失败] 手册目录里没有「数字-名称.md」形态的文件：%~dp0docs & set FAILED=1)
if not defined FAILED if not exist "%~dp0modules\SSHFleet_Go\config\SSHFleet-for-windows.conf" (echo [失败] 找不到出厂配置模板：modules\SSHFleet_Go\config\SSHFleet-for-windows.conf & set FAILED=1)
if not defined FAILED if not exist "%~dp0modules\SSHFleet_Go\config\SSHFleet-for-linux.conf" (echo [失败] 找不到出厂配置模板：modules\SSHFleet_Go\config\SSHFleet-for-linux.conf & set FAILED=1)
if defined FAILED goto :finish

cd /d "%~dp0modules\SSHFleet_Go"
set "OUT=%~dp0build"
if not exist "%OUT%" mkdir "%OUT%"
rem Go 的编译缓存（GOCACHE）走系统默认路径：它可复用、不是产物，不受「构建产物不出工作区」约束。
rem 临时目录反过来必须落在工作区内——那里面装的是编译出来的可执行文件，落在 %TEMP% 下会被
rem 360 当木马静默删掉，现象是构建莫名失败且不报原因
if not exist "%OUT%\tmp" mkdir "%OUT%\tmp"
set "GOTMPDIR=%OUT%\tmp"
set "TMP=%OUT%\tmp"
set "TEMP=%OUT%\tmp"

echo [1/2] 编译 windows/amd64 ...
set GOOS=windows
set GOARCH=amd64
go build -o "%OUT%\SSHFleet.exe" . || (echo [失败] windows/amd64 构建失败 & set FAILED=1)
if defined FAILED goto :finish
echo [完成] build\SSHFleet.exe

echo [2/2] 交叉编译 linux/amd64 ...
set GOOS=linux
set GOARCH=amd64
go build -o "%OUT%\SSHFleet" . || (echo [失败] linux/amd64 构建失败 & set FAILED=1)
if defined FAILED goto :finish
echo [完成] build\SSHFleet

echo.
echo 构建完成，产物：
echo   build\SSHFleet.exe    Windows 可执行文件
echo   build\SSHFleet        Linux 可执行文件

rem ---- 组发布目录 + 打 zip ---------------------------------------------------
rem 打包前先清空整个 release\，目录里永远只有最新一次的产物（见下）

rem Linux 包的**兜底**打包器：系统自带的 bsdtar（首页首选 Git 的 bash，理由见下方 :tar_bash）。
rem 显式写绝对路径——PATH 里若有 Git 的 GNU tar（从 Git Bash 终端调用本脚本时必然如此），
rem 裸 tar 会解析到它，而 GNU tar 把 -f 里的盘符当远程主机名，报
rem 「tar: Cannot connect to D: resolve failed」——2026-09-24 实测踩到
set "BSTAR=%SystemRoot%\System32\tar.exe"
if not exist "%BSTAR%" (echo [失败] 未找到 Windows 自带的 tar.exe（需 Windows 10 1803+），无法生成 tar.gz & set FAILED=1)
if defined FAILED goto :finish

rem 时间戳用作包名（形如 2026-09-24_12-51-27）。取不到就直接报错退出，不留兜底：%DATE% 这类
rem 兜底在中文 Windows 上形如「2026/10/16 周五」，带斜杠与空格，进了包名会建出多层目录、
rem zip 与 tar.gz 的名字跟着乱。包名时间戳的用处就是区分前后两次打包，取不到时间就没有一次
rem 可信的打包，不该带病往下走
set "TS="
for /f %%i in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-dd_HH-mm-ss"') do set "TS=%%i"
if not defined TS (echo [失败] 取不到系统时间，无法生成包名时间戳 & set FAILED=1)
if defined FAILED goto :finish

set "REL=%~dp0release"
set "NAME=SSHFleet_%TS%_windows"
set "PKGDIR=%REL%\%NAME%"
echo.
echo [发布] 组装 release\%NAME% ...
rem 清空前确认 release 确实落在脚本所在目录下。这一句以上 REL 是拼出来的、必然成立，留着是给
rem 日后改这段的自己上的闸：rmdir /s /q 一旦作用到工作区外的目录就是不可逆的
for %%P in ("%REL%") do set "RELPARENT=%%~dpP"
if /i not "%~dp0"=="%RELPARENT%" (echo [失败] release 目录不在工作区内，拒绝清空：%REL% & set FAILED=1)
if defined FAILED goto :finish
rem 打包前清空整个 release\：不留历史包，目录里永远只有最新一次的产物
if exist "%REL%" rmdir /s /q "%REL%"
mkdir "%PKGDIR%\config"
copy /y "%OUT%\SSHFleet.exe" "%PKGDIR%\" >nul
copy /y "%~dp0README.md" "%PKGDIR%\" >nul
copy /y "%~dp0CHANGELOG.md" "%PKGDIR%\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\keywords_dangerous.conf" "%PKGDIR%\config\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\keywords_error.conf" "%PKGDIR%\config\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\keywords_passwd.conf" "%PKGDIR%\config\" >nul
rem 出厂配置按平台取模板，改名为包内的 config\SSHFleet.conf。**不能**拷工作区里那份
rem config\SSHFleet.conf：那是作者本机在用的配置，端口、账号、凭据路径都是他的环境
copy /y "%~dp0modules\SSHFleet_Go\config\SSHFleet-for-windows.conf" "%PKGDIR%\config\SSHFleet.conf" >nul
rem 面向使用者的手册随包（README 那份已在包根，不重复放）。通配 ?-*.md 只取「数字-名称.md」
rem 形态的手册；个人开发规范.md 没数字前缀、天然不进包，以后加一册也不必改脚本。
rem cmd 没有字符类，`?` 只保证「单字符-」的前缀，形态由上面那条前置检查兜着
rem 发布包的 docs\ 与仓库的 docs\ 同名同层，README 里指向手册的链接两边都成立，不必改写
mkdir "%PKGDIR%\docs"
copy /y "%~dp0docs\?-*.md" "%PKGDIR%\docs\" >nul

rem zip 交给 .NET 的 ZipFile（经 powershell 调用）。**不用 Windows 自带的 tar.exe**：
rem 它写 zip 时不给非 ASCII 文件名置 UTF-8 标志（general purpose bit 11），中文 Windows 上
rem 解压工具会按本地代码页（GBK）解，手册的中文名全变乱码——2026-09-24 用 zipfile 核对
rem flag_bits 实测确认（读到的是 0x0008 而非 0x0800）。.NET 会正确置位。
rem 不用 Compress-Archive：它是 Microsoft.PowerShell.Archive 模块里的 cmdlet，从 cmd 调
rem 会「module could not be loaded」；直接调 .NET 类型不依赖任何模块。
rem 另注：bsdtar 不支持 --hdrcharset，GNU tar 不支持写 zip 且会把盘符当远程主机名，
rem 两条路也都走不通
powershell -NoProfile -Command "Add-Type -AssemblyName System.IO.Compression.FileSystem; [System.IO.Compression.ZipFile]::CreateFromDirectory('%PKGDIR%', '%REL%\%NAME%.zip', [System.IO.Compression.CompressionLevel]::Optimal, $true)" || (echo [失败] 生成 zip 失败 & set FAILED=1)
if defined FAILED goto :finish
rem 成功判定再补一道大小：打包中途失败也会留下 0 字节文件，只判退出码会假成功
set "ZIPOK="
if exist "%REL%\%NAME%.zip" for %%F in ("%REL%\%NAME%.zip") do if %%~zF GTR 1000 set "ZIPOK=1"
if not defined ZIPOK (echo [失败] 生成 zip 失败 & set FAILED=1)
if defined FAILED goto :finish

echo [发布] 完成：release\%NAME%.zip

rem ---- Linux 发布目录 + tar.gz（同一时间戳，一次出齐两个平台）--------------
set "LNAME=SSHFleet_%TS%_linux"
set "LPKGDIR=%REL%\%LNAME%"
echo.
echo [发布] 组装 release\%LNAME% ...
mkdir "%LPKGDIR%\config"
copy /y "%OUT%\SSHFleet" "%LPKGDIR%\" >nul
copy /y "%~dp0README.md" "%LPKGDIR%\" >nul
copy /y "%~dp0CHANGELOG.md" "%LPKGDIR%\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\keywords_dangerous.conf" "%LPKGDIR%\config\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\keywords_error.conf" "%LPKGDIR%\config\" >nul
copy /y "%~dp0modules\SSHFleet_Go\config\keywords_passwd.conf" "%LPKGDIR%\config\" >nul
rem 出厂配置按平台取模板（同 Windows 包）
copy /y "%~dp0modules\SSHFleet_Go\config\SSHFleet-for-linux.conf" "%LPKGDIR%\config\SSHFleet.conf" >nul
mkdir "%LPKGDIR%\docs"
copy /y "%~dp0docs\?-*.md" "%LPKGDIR%\docs\" >nul

rem tar.gz 必须保留可执行权限（Linux 解压即用）。System32 的 bsdtar 不支持 --mode；
rem Git 的 GNU tar 需要外挂 gzip（cmd 直接调找不到，且 -f 带 D: 盘符冒号会被当远程主机）
rem ——所以整条 tar 命令交给 Git 的 bash 执行：先把 /usr/bin 提到 PATH 最前
rem （否则 tar 会解析到 System32 的 bsdtar），工作目录继承 pushd 的 release\。
set "GBASH=%ProgramFiles%\Git\bin\bash.exe"
if exist "%GBASH%" goto :tar_bash
rem 兜底：找不到 Git bash 时用系统 bsdtar（绝对路径，理由同 zip 那段），权限位会丢
"%BSTAR%" -czf "%REL%\%LNAME%.tar.gz" -C "%REL%" "%LNAME%"
echo [提示] 未找到 Git 的 bash.exe，tar.gz 里的可执行文件可能没有执行权限，Linux 解压后需执行 chmod +x SSHFleet
goto :tar_check
:tar_bash
pushd "%REL%"
"%GBASH%" -c "export PATH=/usr/bin:$PATH; tar --mode=755 -czf '%LNAME%.tar.gz' '%LNAME%'"
popd
:tar_check
rem 成功判定按大小：tar 中途失败也会留下 0 字节文件，只判存在会假成功
set "TAROK="
for %%F in ("%REL%\%LNAME%.tar.gz") do if %%~zF GTR 1000 set "TAROK=1"
if not defined TAROK (echo [失败] 生成 tar.gz 失败 & set FAILED=1)
if defined FAILED goto :finish
echo [发布] 完成：release\%LNAME%.tar.gz

echo.
echo 全部完成，发布产物（release\）：
powershell -NoProfile -Command "Get-ChildItem -LiteralPath '%REL%' | ForEach-Object { $size = if ($_.PSIsContainer) { '    <目录>' } else { '{0,7:N1} MB' -f ($_.Length/1MB) }; '{0}  {1}  {2}' -f $_.LastWriteTime.ToString('yyyy-MM-dd HH:mm'), $size, $_.Name }"

:finish
if defined OLDCP chcp %OLDCP% >nul
if "%~1"=="" pause
if defined FAILED exit /b 1
exit /b 0
