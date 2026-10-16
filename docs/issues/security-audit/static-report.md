# SSHFleet 安全分析 · 静态报告

> **本报告已按实测结果修订（2026-10-13）。**
> D1 / D2 / D3 三条经实测**推翻，已撤回**（原因见 `live-report.md`）；D4 经开发者确认解释器可自由配置后**升为高危**；D5 定性修正为**呈现层缺陷**而非安全模型问题。
> 本报告的结论以修订后为准，实测证据见同目录 `live-report.md`。

**分析方式**：纯代码阅读，未运行程序。所有结论标注可复核的 `文件:行号`。

**威胁模型**（按开发者交代的实际场景）：

- 本机可靠，不考虑本机被攻陷
- 目标节点**可信**，不考虑恶意主机伪造返回文本
- **脚本或执行目录可能被投毒**——脚本量大，人工看不全
- 2000 个节点，各由不同业务维护，**目标环境高度异构**（这是本工具最主要的不确定性来源）

**本轮覆盖**：`main.go`、`dangercheck/`（全模块）、`ssh/command.go`、`ssh/exec.go`、`ssh/interactive.go`、`ssh/matcher.go`、`ssh/passwdsession.go`、`credential/` 主要文件、`output/archive.go`、`output/output.go`、`cli/check.go` 部分、`config/keywords_dangerous.conf`。

**本轮未覆盖**：`ssh/sftp.go`、`ssh/passwdmatcher.go`、`batch/`、`nodelist/`、`verdict/`、`config/`、`confirm/`、`output/xlsx.go`、`output/report.go`。

**阅读约定**：每条分「代码缺陷」与「设计取舍」两类。缺陷 = 可明确指出改法；取舍 = 需重新评估场景适配或 ADR。严重度按**你的场景**评定，不按通用标准。

---

## 一、代码缺陷

> **修订说明**：本节原有 D1 / D2 / D3 三条，经实测**全部推翻并撤回**。撤回理由摘要如下，完整证据见 `live-report.md`：
>
> - **D1（`xargs` 致命令段被丢弃）——撤回**：读错了 `stripPrefixes` 的逻辑。剥掉 `xargs` 及其选项后剩下的正是真实命令词（`rm -rf`），实测 `echo f | xargs rm -rf` 准确命中"递归强制删除 / 高"，带 `-I{}` 的变体同样命中。
> - **D2（递归深度上限可绕）——撤回**：`maxDepth` 限制的是单次遍历内的**语法树结构深度**，而非间接执行的层数。实测 `eval` 嵌套 15 层、`bash -c` 嵌套 7 层，危险命令均被检出。
> - **D3（解析失败回退致包装剥离失效）——撤回**：shell 家族内该路径不成立——**bash 自己也是先整体解析再执行**，语法错误的命令根本跑不起来，无法构成"检测漏了但真跑了"。实测 `bash -c 'rm -rf x; if'` 报语法错、文件保留。

### D4｜非 shell 解释器的内容检测双向失效

**位置**：`internal/dangercheck/parse.go`（解析层用 sh 语法）+ `internal/ssh/command.go`（`interpreterTable`）

`Analyze` 走 `ParseSource`，而 `ParseSource` 用 `mvdan.cc/sh/v3/syntax`——**始终以 shell 语法解析**。但 `interpreterTable` 支持 **15 种解释器**：

```go
var interpreterTable = []interpreterInfo{
	{"bash", "-c", true, ""}, {"sh", "-c", true, ""}, {"dash", "-c", true, ""},
	{"ksh", "-c", true, ""}, {"zsh", "-c", true, ""}, {"mksh", "-c", true, ""},
	{"ash", "-c", true, ""}, {"python", "-c", false, ""}, {"luajit", "-e", false, ""},
	{"lua", "-e", false, ""}, {"node", "-e", false, ""}, {"nodejs", "-e", false, ""},
	{"perl", "-e", false, ""}, {"ruby", "-e", false, ""}, {"php", "-r", false, ""},
}
```

**经开发者确认：解释器可自由配置，什么脚本都可能。** 两者不对称导致**双向失效**：

| 方向 | 表现 |
|---|---|
| **漏报** | python 的 `os.system("rm -rf /")` 对 shell 解析器只是一行普通命令，不命中任何规则 |
| **误报** | 非 shell 脚本里出现 `rm -rf` 字样（如字符串常量）时，可能被当成真实命令报"高危" |

**实测确证**：`python3 -c "import os; os.system('rm -rf <目标>')"` 无任何警告，目标目录被整个删除；而裸 `rm -rf <目标>` 会命中"递归强制删除 / 高"。

**谁来触发**：脚本或命令内容的作者（你的投毒场景）。

**触发后会发生什么**：危险检测对非 shell 脚本**整体失效**——既拦不住真危险，又可能对无害内容误报。

**严重度**：**高危**。在"什么脚本都可能"的场景下，危险检测只对 shell 脚本有效。

---

### D5｜`--yes` 下 high/medium/low 档放行后，终端无任何提示

**位置**：`main.go` 步骤 5

```go
case len(dangerReport.Matches) > 0:
	if !in.Disinteractive {
		output.PrintDangerWarning(dangerReport, false)
	}
	confirmed, cerr := in.Confirm(common.ConfirmReq{...})
	...
	if in.Disinteractive {
		dangerNote = autoNote   // ← 拼好了，但只进日志
	}
```

**机制**：`internal/cli/interpreter.go:56` 的注释写明"非交互（`--yes`）下 Confirm 直接返回已确认，即'默认为 yes'（2026-10-07 作者定）"。

`forbidden` 档不受影响——它是 `switch` 的**第一个分支**，在 `Confirm` 之前就 `os.Exit(1)`，`--yes` 也拦得住。**这个设计是自洽的**，且开发者已确认 forbidden 档的 10 条规则（`rm -rf /`、`mkfs`、`dd of=/dev/sda`、`shutdown`、`reboot` 等）判断口径合理，把 high 提到 forbidden 会误伤日常操作。

**真正的缺陷在呈现层**：`--yes` 下命中非 forbidden 档时，`autoNote`（"非交互模式放行危险命令（最高级别 X，分类 Y，来源行 N）"）**只由 `Confirm` 写进日志，不打终端**。开发者跑完 2000 台，屏幕上只有"成功 2000"，**无从知晓其中有若干台执行了 `rm -rf`**。

**实测确证**：同一条 `rm -rf <目标>`，交互模式弹警告框 + 询问、文件保留；`--yes` 模式无警告无询问、文件被删。

**严重度**：**高危（场景维度）**。不是安全模型的问题，是"放行了没人知道"。信息已经采集，只是没呈现。

**建议方向**（本轮不改代码）：`--yes` 下命中非 forbidden 档时，在终端打一行摘要（不询问、不阻断）：

```
本次有 N 个节点命中危险命令（最高级别 高），已按 --yes 放行，详见日志
```

**参考实现要点**：`autoNote` 字符串已经在 `main.go` 里拼好，改造成本限于"把它往终端送一次"这一层，不触及危险检测与档位判定。

---

### D7｜`assets/` 清单脱敏依赖「第 1 列必为 IPv4」，失效时静默

**位置**：`internal/output/archive.go`

```go
fields, perr := splitCSVLine(line)
if perr != nil || len(fields) < 4 || !common.IsIPv4Literal(fields[0]) {
	out.WriteString(line + "\n") // 表头、注释、异常行一律原样
	continue
}
```

**当前不构成风险**：`nodelist/fields.go:60` 与 `nodelist/csvread.go:42` 都强制校验 IPv4 字面量，非 IPv4 的清单行在加载阶段就被拒——所以此处判断在正常流程下**恒真**，脱敏不会失效。

**但失效模式是静默的**：一旦将来 IPv4 校验口径松动（例如加入主机名 / IPv6 支持），**脱敏会无声失效**——密码明文写入 `assets/` 副本，且没有任何警告。使用者无从察觉。

**严重度**：低危（当前不可触发，属防御性代码的失效模式）。

---

## 二、设计取舍

### T1｜危险检测的定位（已核查，判定不改）

`GLOSSARY.md` 明写：

> 危险检测（dangerous check）：……定位是操作助手，**不是**拦截恶意命令的防御闸门。

原本担心这个定位与"2000 台 + 脚本可能被投毒"的场景不匹配。经与开发者确认后**判定不必改**：

- `forbidden` 档是唯一 `--yes` 也拦的档，其 10 条规则覆盖的是**"把机器搞死"**级别（`rm -rf /`、`mkfs`、`dd of=/dev/sda`、`shutdown`、`reboot`、`init 0`、`kill -9 -1`、`systemctl mask sshd`），判断口径合理
- 把 `high` 上提到 `forbidden` 会**误伤日常操作**（如 `rm -rf /opt/app/data` 是合法运维动作），工具将无法使用
- 故"只有确定灾难才无条件拦、其余交人判断"的设计**是自洽的**

**真正的问题是"放行了没人知道"**，即 D5，属呈现层而非安全模型。

---

### T2｜脚本检测只覆盖单一文件，不含其引入的内容

`dangercheck/check.go`：

```go
data, err := os.ReadFile(a.Script)
...
src = string(data)
```

只读 `a.Script` 指向的**那一个文件**。脚本内部的 `.` / `source` 引入的其他文件、以及运行期 `curl | bash` 拉取的内容，**均不在检测范围内**。

这是静态检测的固有边界（不做动态追踪就无法覆盖），但值得明确写出——**"检测过"不等于"整条执行链都看过"**。

---

### T3｜主密钥以 `0644` 写入 rc 文件

**位置**：`internal/credential/rcfile.go`

```go
return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
```

主密钥明文写入 `~/.bashrc`（或 `~/.zshrc`），权限 0644。实际安全性由 `~` 目录权限（通常 0755）兜底，属"恰好安全"而非"设计安全"。

**经与开发者确认**：本工具面向运维人员，**不对其本机操作失误设防**。故本条不按风险计。

其余凭据相关实现核查通过：AES-256-GCM + HKDF 标准实现、nonce 随机、日志只打 SHA-256 指纹不打密钥本身、`GuardEncrypt` 在加密前拦密钥不一致。

---

### T4｜代填值与改密回显的保护不一致

**位置**：`internal/ssh/interactive.go` 对比 `internal/ssh/passwdsession.go`

两条路径同样是 PTY、同样开回显、同样把输出写进 `result.Output` 并落盘归档，但**只有改密路径做了脱敏**（`scrubPasswdSecrets`）。

**经与开发者确认**：代填的设计用途是回答"选架构 / 选 deb 或 rpm"这类选项，**不是承载密码**，故不构成缺陷。仅记录落差：若日后代填被用于敏感输入，此处无保护。

开发者已提出后期设想（**是否实现未定，不在本轮范围**）：增加一个"代填内容是否脱敏"的配置开关，默认不脱敏。

---

### T5｜`--no-shell` 完全绕开外壳与环境

`ssh/command.go`：

```go
if noShell {
	if command != "" {
		return command, ""
	}
```

按设计，`--no-shell` 表示"原样下发，工具不附加任何东西"（含 `sudo` 与环境段）。**危险检测在此路径下照常执行**（它分析的是命令原文，与是否套外壳无关），故不构成检测绕过。

**`--sudo` 被忽略这件事有完整提示，不是静默失效**（初稿此处写错，已订正）。`internal/confirm/confirm.go` 有三层保护：

| 位置 | 机制 |
|---|---|
| `confirm.go:110-122` | 专门警告 + 确认询问；`--yes` 下另写 `SkipLine` 留痕 |
| `confirm.go:156` | 参数屏「执行身份」行显示 `登录用户（--no-shell 忽略 sudo）` |
| `confirm.go:167` | 参数屏「解释器」行显示 `原样下发，本次不生效` |

实测输出：

```
⚠ --sudo 与 --no-shell 同时给出
  --no-shell 是原样下发、工具不附加任何东西，所以 --sudo 不生效：
  本次将以【登录用户】身份执行，命令里不会带 sudo。
  要 root 身份，请去掉 --no-shell；要原样跑，请去掉 --sudo。

继续执行？ [Y/n]:
```

---

## 三、附录：低危与已排除项

以下各项经核查**不构成问题**，记录以免重复排查：

| 项 | 结论 |
|---|---|
| 危险规则正则的 `(?i)` 前缀 | 大小写不敏感为有意设计，与旧实现对齐 |
| `cursorMatcher` 零宽命中 | 已处理（`end <= start` 跳过），不会死循环 |
| `driveSession` 事件通道阻塞 | **不成立**：每条规则至多命中一次，事件总数 ≤ 规则数，容量充足 |
| 交互分支 base64 正文 | **安全**：base64 字符集不含单引号，`shellQuote` 转义充分 |
| 脚本名 `$0` 注入 | **安全**：`scriptNameArg` 经 `shellQuote` 包裹 |
| 环境段值注入 | **安全**：`quoteEnvValue` 双引号包裹 + 配置期 `validateEnv` 拦尾部反斜杠 |
| `assets/` 代填文件明文 | **非缺陷**：注释明示按"留痕优先"口径，与 report、工具日志同一口径 |
| 清单脱敏（IPv4 清单） | **有效**：第 4/6 列经 `MaskSecret` 处理 |
| `kill -9 -1` 等强规则 | 规则文件语义与旧实现一字不动 |

---

## 四、实测结果（已完成）

下列各项已在本机测试环境实测完毕，**详细证据与命令见 `live-report.md`**：

| 编号 | 待验内容 | 结果 |
|---|---|---|
| V1 | `echo "rm -rf /" \| xargs` 是否真绕过检测 | ❌ 推断错误，正常命中 |
| V2 | 深层嵌套是否绕过深度上限 | ❌ 推断错误，15 层仍检出 |
| V3 | 何种语法错误会让解析回退且 shell 仍可执行 | ❌ shell 家族内不成立 |
| V4 | `python3 -c "os.system('rm -rf ...')"` 是否被检测 | ✅ **确证：不被检测，命令真的执行了** |
| V5 | `--yes` 下 high 档命中是否静默放行 | ✅ **确证：无警告无询问，文件被删** |
| V6 | 清单非 IPv4 行是否在加载阶段被拒 | 间接确认（IPv4 为强制校验） |

**撤回项已于本报告第一节顶部标注**；D4 升为高危、D5 定性为呈现层缺陷。

---

## 五、总体判断

在**你交代的威胁模型**下，本工具的核心风险**不在"外部攻击"**——它没有 HTTP 接口、不监听端口、本地几乎不执行命令（全仓仅 `credential/masterkey_windows.go` 一处 `exec.Command`，调 `setx`）。

真正的风险集中在两处（均已实测确证）：

1. **危险检测对非 shell 脚本整体失效**（D4）——工具支持 15 种解释器且可自由配置，但检测层只用 shell 语法解析。**漏报真危险，还可能误报无害内容。**
2. **放行了但你看不见**（D5）——`--yes` 下 high/medium/low 档静默放行且终端无摘要。你跑 2000 台看到的是"成功 2000"，无从知晓其中有若干台执行了 `rm -rf`。**信息已采集，只是没呈现。**

**凭据保护是这条代码里做得最扎实的部分**：加解密实现标准（AES-256-GCM + HKDF、nonce 随机）、日志只打 SHA-256 指纹不打密钥本身、归档清单脱敏、改密路径主动擦除回显。**没有发现凭据明文泄漏的确凿路径。**

**危险检测的定位问题**（原 T1）经与实际场景对照后**不必再改**：`GLOSSARY.md` 说它是"操作助手"而非"防御闸门"，而开发者确认 `forbidden` 档（唯一 `--yes` 也拦的档）覆盖的正是"把机器搞死"级别，判断口径合理、不应上提 high。**D5 的问题不在档位，在提示。**

---

**报告日期**：2026-10-13（修订：同日，按实测结果）
**分析者**：AI Agent（静态阅读 + 实测）
**复核方式**：每条均可按 `文件:行号` 独立核对；实测部分按 `live-report.md` 第五节命令复现
