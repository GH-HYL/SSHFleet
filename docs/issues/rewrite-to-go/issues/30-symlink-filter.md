# M7-30 软链接双向过滤：过滤掉但要说清，过滤后为零才报错

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 29

## 背景

用户 2026-09-14 裁定：软链接在上传与下载两侧**一律过滤**，但**过滤后一个可传文件都不剩时必须报错**；同时不能悄悄过滤——
被过滤的条目要**写进该节点的 `output` 字段**（成功/失败计数所在的那段文本）；`-u` **额外**在**终端**提示
（上传源在本地、参数一输入即可知；下载源要连上服务器才知道，故终端提示是上传独有）。

**依据（旧实现的终端约定，2026-09-14 复查）**：传输模式的终端**只有进度条**，结果明细只落 `output.txt`。
旧 Python `record_result_output` 里写着 `if not args.u and not args.d: console.print(formatted)`；旧 Go 引擎全仓只有一处
`fmt.Println`（日志初始化失败）。所以 `-u` 的终端提示是**额外**加的一条通道，不是恢复旧行为。

## 改动前的实测行为（复现，非推断）

| 场景 | 改前 | 改后 |
| --- | --- | --- |
| `-u` 目录（3 真文件 + 2 软链接） | 静默跳过，只传 3 个，**无任何提示** | 同左过滤，终端提示被过滤的 2 个 |
| `-d` 目录（3 真文件 + 2 软链接） | `find -type f` 直接不列软链接 → **毫无痕迹**，`total_files=3` | 过滤 + 写进 output 明细 |
| `-u` 路径本身是软链接 | 报「-u 参数指定的上传文件或目录是符号链接」（第 5 步已有） | 不变（第 5 步拦下，措辞更早更清楚） |
| `-d` 目标全是软链接 | 报「远程目录为空」（**原因不对**） | 「没有可下载的文件：目标全部为软链接（已过滤 N 个）」 |
| `-d` 空目录 | 「远程目录为空」 | 「远程路径中没有可下载的文件」（含义准确；两者本就同属分类「远程无文件」，分类不变） |

## 顺带修掉的 Windows 专属缺陷

**目录联接（junction）被当成普通文件上传。** Go 1.23 起 junction 被标为 `ModeIrregular`，且 `IsDir()` 为 false、`ModeSymlink` 为 false（同 spec D47 的踩坑）。采集器旧判断只认 `ModeSymlink`，于是 junction 走到「当作文件」分支 → `os.Open` 打开目录成功 → 当成待传文件 → 传输时读目录失败 → **该节点整台上传失败**。
现在统一走 `isLinkish()`：`ModeSymlink` 直接判链接；`ModeIrregular` 时用 `os.Readlink` 复核（只认真能读出目标的，避免误判其它非常规文件）。

## 实现

| 文件 | 改动 |
| --- | --- |
| `internal/batch/collect.go` | `CollectLocalFiles` 返回 `*CollectResult{Files, Skipped}`；链接类条目（软链接 / junction / `.lnk`）记入 `Skipped` 并过滤；仅当 `Files` 为空时按「全是链接」或「目录为空」分别报错；新增 `isLinkish`、`Summarize` |
| `internal/batch/batch.go` | `Hooks` 新增 `OnNotice`；`task` 新增 `skipped`；`buildTasks` 返回提示文案；`Run` 在**任何进度事件之前**下发提示 |
| `internal/ssh/sftp.go` | 上传侧 `UploadFiles` 接收 `skipped`，把它作为明细行写进 `output` 字段（与下载对齐，不计成功/失败）；下载侧一次 `find` 同时枚举真文件（`F`）与软链接（`L`）；远端路径本身是链接时直接过滤；`totalFiles` 语义保持「可传文件数」；`test -e` 与 `find` 的远端路径统一走 `escapeShellArg` |
| `main.go` | 进度界面改为**首个进度事件时才创建**，保证采集期提示先落终端、不被光标上移重绘覆盖；`OnNotice` 打印到 stdout |

## 验证

**真机（172.28.118.49）— 下载侧（Windows 二进制）**

- 混合目录 → 3 个真文件落地、2 个软链接写入 output：`link_a.txt: 已跳过（符号链接）`、`link_sub: 已跳过（符号链接）`
- 目录内全为软链接 → `没有可下载的文件：目标全部为软链接（已过滤 2 个）`
- `-d` 路径本身是软链接 → 同上（已过滤 1 个）
- 空目录 → `远程路径中没有可下载的文件`（分类仍为「远程无文件」）

**WSL Ubuntu-26.04（真 Linux 软链接）— 上传侧（Linux 二进制）**

- 目录（3 真文件 + 2 软链接）→ 终端在进度条**之前**打印
  `提示：上传源中有 2 个软链接/快捷方式被过滤（不上传）：link_to_dir、link_to_file.txt`，
  且未被进度重绘覆盖；远端只落了 3 个真文件、无软链接
- 同一次执行的归档里，该节点 `output` 字段（`SSHFleetExec.log` 与 `output.txt` 内容一致）：
  `total_files=3, success_files=3, failed_files=0` 之后是
  `link_to_dir: 已跳过（符号链接）`、`link_to_file.txt: 已跳过（符号链接）`，再是三个上传成功行
- 下载侧回归（Windows 二进制，2 真文件 + 1 软链接）：2 个落地、`link_a.txt: 已跳过（符号链接）` 入 output
- 目录内全为软链接 / `-u` 路径本身是软链接 / 目录内无真文件 → 均被第 5 步参数合规检查拦下，文案明确
- 无软链接的正常目录 → 不出现过滤提示（不误报）

**Windows（本机）— 目录联接**

- 用工具自身在该目录内造出 `latest_history`（Python 读原始 reparse tag = `0xA0000003`，`os.path.islink` 为 false，确认是 junction 而非符号链接）
- `-u` 该目录 → 提示 `…1 个软链接/快捷方式被过滤（不上传）：latest_history`，不再当作文件上传

## Comments

- 2026-09-14 完成。语料/单元测试未新增（本项属 I/O 行为，靠真机 + 真 Linux 双侧验证）；`go test ./internal/...` 全绿，`gofmt`/`go vet` 干净，`windows/amd64` 与 `linux/amd64` 双目标编译通过。
- 一处已知取舍：非 sudo 上传的远端中间目录用 SFTP `MkdirAll`，root 用户跳过 sudo（沿用旧行为），本次未动。
