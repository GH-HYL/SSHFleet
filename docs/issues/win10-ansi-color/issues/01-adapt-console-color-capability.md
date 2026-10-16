# 01: 颜色能力由程序自己适配（探测 + 开 VT + 降级）

Status: resolved（2026-10-16 实施，提交 `ea5cec1`）
Blocked by: 无

**对应规格**：`../spec.md`「实施决定」「验收」两节。

---

**本议题只有这一条工单。** 三件事**必须一起做**：探测能力、主动开 VT、颜色取值跟着探测走。

拆开会每半都不完整——只开 VT，则"是控制台但打不开 VT"的环境仍外泄；只改常量取值，探测结果无处产生。
合起来是一条从「探测」到「取值」到「输出」的竖切，做完即可按下面「验收」一节逐条核对。

---

## 要做的事

### 1. 新增：颜色能力探测（`internal/common`，平台分文件）

| 文件 | 构建约束 | 内容 |
| --- | --- | --- |
| `internal/common/color.go` | 无 | `var ColorEnabled = detectColor()`；`func ANSI(code string) string`（不启用时返空串） |
| `internal/common/color_windows.go` | `//go:build windows` | `detectColor()` 三步：取控制台句柄**失败** → 不是控制台，按「输出颜色」返 `true`（本议题不碰该场景）；取到且 VT 位已开 → 返 `true`；取到但未开 → `SetConsoleMode` 打开，成返 `true`、败返 `false` |
| `internal/common/color_other.go` | `//go:build !windows` | `detectColor()` 恒返 `true`（**行为不变**，见规格「不在本次范围」） |

**三步顺序不许改**，也不许合并成"先试开、失败再判"这类二次探测。
**MUST NOT** 在探测里输出任何东西（否则会早于探测结果自己先吐了颜色）。

### 2. 改动：26 个颜色常量 `const` → `var`

值改经 `common.ANSI(...)` 取。`internal/common` 自己的三个常量在包内直接调 `ANSI(...)`，不带包名前缀。

| 文件 | 常量数 | 常量名（2026-10-16 位置，实施时以符号为准） |
| --- | --- | --- |
| `main.go` | 3 | `colorReset` `colorRed` `colorYellow` |
| `internal/common/interactor.go` | 3 | `colorReset` `colorRed` `colorYellow` |
| `internal/confirm/confirm.go` | 8 | `colorReset` `colorBold` `colorCyan` `colorYellow` `colorBlue` `colorBrightYellow` `colorBrightCyan` `colorBrightOrange` |
| `internal/credential/masterkey.go` | 2 | `colorReset` `colorYellow` |
| `internal/nodelist/fields.go` | 3 | `colorReset` `colorCyan` `colorYellow` |
| `internal/output/terminal.go` | 7 | `ansiReset` `ansiGreen` `ansiCyan` `ansiRed` `ansiDim` `ansiYellow` `ansiWhite` |
| **合计** | **26** | |

六个文件各一个 `const` 块。**MUST NOT** 顺手重命名、**MUST NOT** 调整常量顺序、**MUST NOT** 合并或拆分这些块。

### 3. 不动：145 处引用点

全是字符串拼接，值变空串后行为自然退化。**一处也不许改。**

### 4. 不动：进度界面

`internal/output/progress.go` 走 lipgloss，对上述常量引用数为 0。**本次不含它。**

---

## 验收

判据与命令见规格「验收」一节。逐条勾：

- [x] `go build ./...` 通过；`go vet ./...` 干净
- [x] 非 Windows 分支能编译（`color_other.go` 生效）——`GOOS=linux go build ./...` 通过
- [x] **改前基线**：`probe7` 跑 Go 版——跑之前 VT=false、跑之后 VT=false、读回含 ESC=true（实测见 Comments）
- [x] **改后**：同一命令——跑之后 VT=true、读回含 ESC=false（实测见 Comments）
- [x] **重定向行为未变**：`printf 'n\n' | ./SSHFleet.exe … > raw.txt 2>&1`，`raw.txt` 里 ESC 计数仍为 48（与改前一致）
- [x] 26 个常量已全部改为 `var` 并接上 `ANSI(...)`（逐文件核对，等于上表）
- [x] 145 处引用点零改动（`git diff` 里不含任何调用点）
- [x] 本次不写测试（作者指示：「测试的事儿现在不用你管」）
- [x] CHANGELOG 已按规范第四节记（`[9.1.6] - 2026-10-16`，修订档）

**证据要求**：改前 / 改后各跑一遍，把两次实际输出贴进本工单 `Comments`。
只写「已修复」不算交付（规范第五节「交付要给证据」）。

---

## 边界（越界即停）

```text
- 不碰输出重定向 / 管道场景（作者指示：「不用考虑重定向的事情」）
  —— 探测取不到控制台句柄时按「输出颜色」处理，行为与现在一致
- 不动非 Windows 平台的行为
- 不做输出流收敛、不引入新依赖、不做真彩色探测
- 不动进度界面配色
- 本次不设测试接缝、不写测试
- 红线 4：以上之外顺手发现的问题，只报告不改
```

## Comments

**2026-10-16 · 已实施，提交 `ea5cec1`。** 三件事一次做完（探测 + 主动开 VT + 取值跟着探测走），引用点零改动。

改前基线（`build/atest-8/`，`./probe7.exe ./fleet/SSHFleet.exe -c "echo hi" -f ./fleet/nodes.csv`）：

```text
=== 跑之前  mode=0x0003  VT-OUT=false（已强制关闭）
=== 跑之后  mode=0x0003  VT-OUT=false
=== 【判定】它没有动 VT
--- 读回第 0 行 ---
"\x1b[31m[ERROR]\x1b[0m\x1b[33m [function:config]\x1b[0m 加载配置文件失败：config/SSHFleet.conf"
=== 读回内容含 ESC(U+001B) = true
```

改后（同一条命令）：

```text
=== 跑之前  mode=0x0003  VT-OUT=false（已强制关闭）
=== 跑之后  mode=0x0007  VT-OUT=true
=== 【判定】是它把 VT 打开的
--- 读回第 0 行 ---
"[ERROR] [function:config] 加载配置文件失败：config/SSHFleet.conf"
=== 读回内容含 ESC(U+001B) = false
```

重定向行为未变（`cd build/atest-8/fleet`）：

```text
$ printf 'n\n' | ./SSHFleet.exe -c "echo hi" -f nodes.csv > ../raw_after.txt 2>&1
$ grep -o $'\x1b' ../raw_after.txt | wc -l
48
```

改前同一命令同样是 48 次——本议题不碰该场景，行为与改动前一致。

构建与静态检查：

```text
gofmt -l .                    无输出
go build ./...                OK
go vet ./...                  OK
GOOS=linux go build ./...     OK
```

改动范围：`git diff --stat` 六个文件（38+/32-），全部落在上表那六个常量块内；145 处引用点一处未碰。
改前的二进制留档在 `build/atest-8/fleet/SSHFleet_before.exe`，可复跑上面对照。
