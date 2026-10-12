# M6-24 构建脚本路径修正 + 跨平台编译修复

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 23

## 背景

做工单 23 的跨平台验证时（`GOOS=linux go build ./...`），连带查出三个既有缺陷。

## 缺陷一：`build-linux.sh` 产物落错目录

`cd "$(dirname "$0")/modules/SSHFleet_Go"` 之后用 `OUT=../build`，实际解析成 `modules/build`——比约定深一层。

**这同时更正了 M1 的一次误判**：当时看到 staging 里混进 `modules/build` 两个 8MB 产物，归因于「损坏版 bat 的残留」，实际就是这个脚本自己写的。

修法：`ROOT="$(cd "$(dirname "$0")" && pwd)"`，按脚本自身位置定位工作区根。

## 缺陷二：同一脚本在 Git Bash 下把产物写到 `D:\d\…`

修完缺陷一又踩：Git Bash 的 `pwd` 给 `/d/Desktop/…` 形式，而 `go.exe` 是原生 Windows 程序，把 `/d/…` 当成「当前盘根下的 d 目录」，产物于是落到 `D:\d\Desktop\Code\SSHFleet\build\`。

修法：`command -v cygpath` 存在时用 `cygpath -m "$ROOT"` 转成 `D:/…` 混合形式再交给 go；真实 Linux 上无 cygpath，直接用 ROOT。（与坑表 `go build -o /tmp` 同源。）

## 缺陷三（影响更大）：Linux / macOS 根本编译不过

`internal/credential/masterkey.go` 无条件 import `golang.org/x/sys/windows/registry`，`GOOS=linux|darwin go build ./...` 直接失败。M2 引入，**直到本次才发现**（此前每期只跑 Windows 侧 `go build ./...`）。

修法：按平台拆分。

| 文件 | 内容 |
| --- | --- |
| `masterkey.go` | 平台无关：`envName`、`GetMasterKey`、`GenKey` |
| `masterkey_windows.go` | `readPersistedKey`（注册表直读）+ `persistKey`（setx） |
| `masterkey_unix.go` | `readPersistedKey`（查 `~/.zshrc`、`~/.bashrc`）+ `persistKey`（按 `$SHELL` 选 rc 文件）+ `findExportInFile` |

## 顺带

`.gitignore` 白名单的 `!modules/**` 会连带放行 `modules/build/`（正是缺陷一被误入库的机制），补一条显式排除。

提交时 git 报出「LF will be replaced by CRLF」——本机 `core.autocrlf=true` 来自 Git for Windows 的**系统级默认**（`C:/Program Files/Git/etc/gitconfig`），checkout 时会把 `build-linux.sh` 变成 CRLF，而它存在的唯一目的就是供 Linux / 自动化使用（Linux 与 Git Bash 都会因 `bash\r` 失败）。故新增 `.gitattributes` 把换行符按用途固定：`.sh` 强制 LF、`.bat` 强制 CRLF。已核对：仓库内 `build-windows.bat` 的 UTF-8 BOM（`EF BB BF`）完好保存（BOM 属内容，不受换行符规范化影响）。

## 验证

- `gofmt -l .` 空；`go vet ./...` 无输出
- 四个目标全部编译通过：`windows/amd64`、`linux/amd64`、`darwin/arm64`、`windows/arm64`
- `./build-linux.sh` 重跑：产物落**工作区根** `build/`（ELF 64-bit + PE32+ 各一）；`modules/build/` 不再出现；误建的 `D:\d` 树已清除
- `go test ./internal/...` 全绿（危险 81/81、分类 24/24）

## Comments

- 2026-09-14 完成。**新规矩**：每次动代码后都要跑一次 `GOOS=linux GOARCH=amd64 go build ./...`（已写入 HANDOVER 第三节与第六节坑表）。
- `build-windows.bat` 未受影响：它用 `%~dp0build`（原生 Windows 路径），本身是对的；但本环境 cmd 被沙箱禁用，无法再执行验证，仅作静态确认。
