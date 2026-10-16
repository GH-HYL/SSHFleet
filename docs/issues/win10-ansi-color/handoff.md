# 交接：客户机 Win10 上 ANSI 颜色显示异常

Status: needs-info

> **⚠️ 本文若干前提已被下一轮实测推翻，读前先看 `findings-2026-10-16.md`。**
> 已被推翻的三处：
> - 第 4 节「复现环境只在客户机上，本地验证这条路走不通」——**错误前提**，本机即可复现；
> - 第 3 节第 4 条的「参数屏早于 rich 控制台创建」——**时间关系反了**，rich 是模块级创建，早于参数屏；
> - 第 5 节第 2 项的 `0x9a` 怀疑——**误报**，那是 `：` 的 UTF-8 末字节。
>
> 另：第 2.1 节把附件 03 读作「同一环境下两种进程表现不同」——**这个读法是对的**，原因不是 rich 而是 loguru。
> 第 5 节第 1 项（谁给 Python 版开的 VT）已定案：**loguru 经 colorama 在 import 阶段开的**，
> 落在 `colorama/winterm.py:185`；调用链见 `findings-2026-10-16.md` 第 4 节。
>
> **本目录的三份文档各管什么**：
>
> | 文件 | 地位 |
> | --- | --- |
> | 本文件 | 上一轮的交接背景。多处前提已被推翻，**只当历史看** |
> | `findings-2026-10-16.md` | 本轮实测证据（事实依据），根因定案在这里 |
> | `spec.md` + `issues/01-*.md` | 实施依据与工单（方案 C：主动开 VT + 拿不到就降级） |
> | `docs/adr/0017-program-adapts-console-not-user.md` | 决策记录：适配由程序做，不推给使用者 |

> 本文是**交接背景**，不是结论。上一轮全程只读排查，**没有动过任何代码**，也没有形成定论。
> 其中若干推测已被证伪，一并记下以免下一轮重复走弯路。

---

## 1 问题是什么

在**客户机（不是开发者的机器）**的 Windows 10 上跑 Go 版 `SSHFleet.exe`，本该上色的地方没有上色，屏幕上出现的是**转义序列的残骸**，例如：

```text
【96m▶ 权限类型  -→【0m   【38;5;214msudo【0m
【93m是否执行上述参数？【0m 【31m[Y/n]【0m:
```

即 `ESC`（0x1B）本身没能生效，终端把它画成一个替代方块，后面的 `96m`、`0m`、`38;5;214m` 被当作普通文字接着打出来。

见附件 `attachments/01-go版参数确认屏乱码.jpg`。

**开发者自己的 Win10 上没有这个问题。** 目前只在客户机上观察到。

---

## 2 现场材料（已留存）

| 文件 | 内容 | 来源 |
|---|---|---|
| `attachments/01-go版参数确认屏乱码.jpg` | Go 版参数确认屏，转义序列以 `【96m` 形态外泄 | 客户机，开发者提供 |
| `attachments/02-客户机系统版本.jpg` | 客户机 `systeminfo`：Windows 10 专业版 **10.0.18363**（1909），64 位 | 客户机，开发者提供 |
| `attachments/03-客户机对照实验.png` | **关键证据**，上下两个 PowerShell 7 窗口，见下节 | 客户机，开发者提供 |

原图另存于 DSH 附件库（只读副本），sha256 分别为：

```text
01  f14df6b939f6e94d829db5da96907984bc23e726eff480f9dc118532c0080ab4
02  60b30b66bb88d3360c892d499652172d2de1fab7dc3e61de590a778e7c1df113
03  4cd4070fe063a2dcaeb58d35ff0ba4d49e0b597c1a3ad8e7cf3795f16c8ca838
```

### 2.1 附件 03 的读法（重要）

同一台客户机、同一个 PowerShell 7 环境下，**两种进程表现不同**：

```text
上半窗口：python -c "print('\033[96m亮青色测试\033[0m')"
          → 输出 【96m亮青色测试【0m          ← 未解析

下半窗口：PyInstaller 打包成的单文件 .SSHFleet.exe
          → 其中 [ERROR] 前缀正常上色         ← 已解析
```

**这是目前唯一一条「同一环境下有正常案例」的现场证据**，也是后续排查的主要抓手。
（曾一度误读为「两处都乱码」，经开发者指出后更正。）

---

## 3 已经排除 / 已经否定的方向

**以下几条在上一轮被讨论过，但已被证据推翻，下一轮不必重走：**

1. **「换一种兼容性更高的颜色字符/色号」——无效。**
   现场证据里连最基础的 `\033[31m`（ANSI 元老级色号）也同样是残骸。如果只是「不支持 256 色」，应当是部分颜色正常、部分异常，而不是全线崩溃。所以问题不在色号新旧。

2. **「换 shell（装 pwsh）能解决」——未解决。**
   开发者已装 PowerShell 7，现象依旧。截图里两个窗口都是 PowerShell 7 启动的。

3. **「Python 版没问题是因为 Python 更兼容」——不成立。**
   客户机上裸 `python -c` 打印 ANSI 同样未解析（附件 03 上半）。

4. **「是 rich 给传输模式专用，与参数屏无关，所以 rich 无关」——此判断出现过，但未能证实现场。**
   开发者指出参数屏早于 rich 控制台创建；同时客户机 PyInstaller 包里**内置了 rich**（客户机系统本身没装 rich，是打包进 exe 的）。
   「rich 是否参与开 VT」目前**仍未被证实或证伪**，属于待查项（见第 5 节）。

5. **「用注册表 / 环境变量排查」——未执行。**
   上一轮曾拟过一组诊断命令，但未在客户机上跑过。

---

## 4 目前掌握的环境事实

```text
开发者机器（双系统 Ubuntu + Win10）
  - Win10 上装了 pip rich（开发环境）
  - 在这台机器上【未复现】该问题
  - 曾用 PyInstaller 把 Python 版打包成单文件分发

客户机（问题发生地）
  - Windows 10 专业版 10.0.18363（1909），64 位
  - 没装 rich；拿到的是 PyInstaller 打包的单文件 exe
  - 使用 PowerShell 7 启动程序
  - Go 版 exe 与裸 python 的 ANSI 均未解析；PyInstaller 打包版却正常上色
```

**注意：复现环境只在客户机上。** 开发者自己的 Win10 无法复现，因此本地验证这条路走不通，验收也必须回到客户机。

---

## 5 下一轮建议继续查的方向（均为未证实推测，勿当结论用）

按「先取证据、再下判断」的顺序排列：

1. **确认客户机上 PyInstaller 版究竟是谁开的 VT。**
   客户机没装 rich，所以不能直接 `import rich` 测。可行的替代路子：
   - 让客户机跑其他**同类打包产物**做对照（换一个不带 rich 的 PyInstaller 包）；
   - 或者请开发者在自己 Win10 上取 rich 的真实源码/版本，看其中是否存在 `SetConsoleMode` / `ENABLE_VIRTUAL_TERMINAL_PROCESSING` 的调用。

2. **确认客户机的终端类型与启动方式。**
   是从已开好的 PowerShell 7 窗口里跑，还是双击 exe？双击会起一个全新的 conhost，二者继承的状态可能不同。
   两张截图的窗口样式看着并不一致（一张带汉堡菜单、黑底，一张是传统 conhost 外观），值得让开发者确认。

3. **确认客户机上是否已有别的程序/设置动过 VT。**
   例如注册表 `HKCU\Console\VirtualTerminalLevel`、`ANSICON` / `ConEmuANSI` / `TERM` 之类环境变量。

4. **确认「客户机 1909 + conhost」这条路径上，Go 版 exe 与 PyInstaller 版 exe 的进程级差异。**
   关键问题：是不是「有某个进程在启动时主动开了 VT」，而 Go 版没有任何人做这件事。

5. **核实 Go 版是否存在与颜色无关的独立缺陷。**
   附件 03 的 PyInstaller 版输出中还出现了：

   ```text
   异常类型：<class 'UnicodeDecodeError'>
   异常信息：'utf-8' codec can't decode byte 0x9a in position 1: invalid start byte
   ```

   `0x9a` 不是合法 UTF-8 起始字节，怀疑与清单文件编码有关。**这与颜色问题无关，建议单独跟踪**，但需确认 Go 版是否同样存在。

---

## 6 代码侧已核实的现状（供下一轮参考，非结论）

上一轮为估算成本，对 Go 代码做过只读核查：

- ANSI 颜色常量共 **26 个**，全部是纯字符串常量，分散在 **6 个包**：
  `internal/confirm`（8）、`internal/output`（7）、`internal/common`（3）、
  `internal/credential`（2）、`internal/nodelist`（3）、`main.go`（3）。
- 引用形态高度统一，全是 `fmt` 拼接（`%s...%s`），未见结构化样式对象；
  引用点合计约 150 处。
- `internal/output/reporter.go:512` 已有 `isatty` 判断（`mattn/go-isatty`），但只覆盖 output 的一部分。
- `golang.org/x/sys/windows` **已在 `go.mod` 依赖中**（第 16 行）。
- 仓库已有平台分文件先例：`internal/output/link_windows.go`（`//go:build windows`）。
- 进度界面走 bubbletea + lipgloss，自成一套配色，与本议题的关系尚未厘清。

**以上仅用于判断改动规模，不代表已选定任何方案。**

---

## 7 工作区规矩（下一轮必读）

- 仓库内 `docs/个人开发规范.md` 是最高约束；`AGENTS.md` 只做导航。
- **红线 1：未获「授权开工」，不得写/改代码。** 上一轮自始至终只读，未越界。
- **红线 5：改缺陷前必须先有复现手段**（指得出已跑过、能当场重现的命令）。当前复现只在客户机上。
- 派子 agent 时，指令里必须写明「本轮已获授权开工」或「只做只读分析」。
- 产物不得出工作区；`*_bak*/` 为只读旧工程，不得修改。

---

## 8 建议下一轮加载的技能

| 技能 | 何时用 |
|---|---|
| `/diagnosing-bugs` | 继续排查本问题时的主流程 |
| `/grilling` | 若需在客户机取证方案之间做取舍、或目标再次变模糊时 |
| `/to-spec` | 若排查收口、要固化成一则规格时 |
| `/implement` | 规格落定、且已获授权开工之后 |

---

## 9 关联材料

- Python 版历史工程（只读参考）：`/home/hyl/桌面/Code/Old-Code_Bak/SSHFleet/modules/SSHFleet_Py`
  - 颜色常量现在 `src/common/constants.py` 第 9–19 行（注释注明由原 `src/color.py` 并入）
  - 参数屏渲染：`src/input/confirm.py` 的 `_print_info_table`（裸 `print` + 裸 ANSI，未用 rich）
  - rich 仅用于 `src/output/terminal.py`（进度/传输界面）
- PyInstaller 打包产物：`.SSHFleet.exe`（在客户机上运行，**仓库内没有此文件**）
