# M6-25 build-windows.bat 加固（cd 守卫 / go 前置检查 / 暂停开关 / 代码页还原）

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 24

## 背景

用户要求复查 bat 版本是否有同类问题（工单 24 刚修完 sh）。

## 先说结论：功能本身没问题（已实测）

- 双产物正确落工作区根 `build\`，且与 sh 的产物 **md5 完全相同**
- 编码 UTF-8 BOM + CRLF 正确；`setlocal` 兜住 `GOOS`/`GOARCH` 不外泄；stderr 干净

## 修掉的一个真缺陷：`cd` 失败不检查

**实测**：把 bat 拷到没有 `modules\SSHFleet_Go` 的隔离目录，并把工作目录设成另一个 Go 包目录后运行——

- 旧版照常报「[完成]」两次 + 「构建完成!」，产物却是**那个别的包**编出来的
- 唯一痕迹是 stderr 一行 `The system cannot find the path specified.`，不阻断执行
- 双击场景下 cwd 是脚本所在目录（工作区根，没有 `.go` 文件），会误报「windows/amd64 构建失败」——指向错误的原因

修法：`cd` 之前先 `if not exist "%~dp0modules\SSHFleet_Go\"` 守卫（用目录存在性判断，不依赖 `errorlevel` 语义）。

## 三处加固

| 位置 | 原状 | 改法 |
| --- | --- | --- |
| 三处 `pause` | 无条件执行 → 非交互调用挂死（本次实测要预先喂 stdin；M1 也为它绕过好几轮） | `if "%~1"=="" pause`：双击（无参数）行为不变，自动化传任意参数即可跳过 |
| 构建前 | `go` 不在 PATH 时报「构建失败」，误导为代码问题 | 前置 `where go` 检查，明确提示「未找到 go，请先安装 Go 并加入 PATH」 |
| 两处 `if %errorlevel% neq 0` | 当前**正确**（该行与括号块在 `go build` 之后才解析展开），但属脆弱写法：以后挪进别的块或前面加 `&` 串联就会读到旧值 | 改用 `cmd \|\| (...)` + `if defined FAILED goto :finish`，不做变量展开 |
| 第 4 行 `chcp 65001` | 不受 `setlocal` 约束，从既有窗口调用会在脚本结束后遗留代码页 | 先读原代码页（**必须在切 65001 之前读**，否则读到的是刚设的值）再切换，收尾还原 |

## 验证（真机执行，逐条对照）

| 用例 | 期望 | 结果 |
| --- | --- | --- |
| 正常（带参数） | 构建成功、**不**暂停 | `[完成]`×2 + 「构建完成!」，无暂停提示，ExitCode=0 |
| 无参数（等价双击） | 构建成功 + **保留**暂停 | 同上 + `Press any key to continue` |
| 隔离目录 + cwd 为别的 Go 包 | **失败且不产出** | `[失败] 找不到子模块目录：...`，隔离目录无 `build/`，ExitCode=1，stderr 无残留 |
| PATH 里没有 go | 明确提示 | `[失败] 未找到 go，请先安装 Go 并加入 PATH`，ExitCode=1 |
| 与 sh 的等价性 | 产物一致 | 两脚本产物 md5 相同 |

## Comments

- 2026-09-14 完成。**踩到一个新坑**：文件工具覆写 `.bat` 时会**保留原文件的 UTF-8 BOM**，我又手工补了一遍，得到双 BOM（`ef bb bf ef bb bf 40`）——cmd 吃掉第一个后，第二个黏在 `@echo` 上，报 `'@echo' is not recognized`，而中文显示却正常（因为文件确实是 UTF-8，具有迷惑性）。已改为「先剥掉全部 BOM 再补一个」，并把这坑记进 HANDOVER。
- 未改动语义：仍先编 windows 再编 linux（与 sh 的顺序相反，但产物一致，属无害差异），仍以 `dir "%OUT%" /T:W` 收尾。
