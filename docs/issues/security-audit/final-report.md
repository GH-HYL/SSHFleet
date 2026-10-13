# SSHFleet 安全分析报告

**分析对象**：SSHFleet（Go 版，`modules/SSHFleet_Go`，96 个源文件约 17500 行）
**分析方式**：代码阅读 + 本机实测
**测试环境**：Ubuntu 本机，独立 sshd 实例监听 `127.0.0.1:2222`，测试账号 `sshfleet_test`
**报告日期**：2026-10-13

---

## 一、结论摘要

**在既有威胁模型下**（本机可靠、目标节点可信、脚本可能被投毒、2000 台环境异构），本工具**不存在可被外部攻击的入口**：它没有 HTTP 服务、不监听端口、本地几乎不执行命令（全仓仅 `credential/masterkey_windows.go` 一处 `exec.Command`，用于 Windows 下调 `setx`）。

**凭据保护是这条代码里做得最扎实的部分**：加密实现标准、日志不泄密钥、归档清单脱敏、改密路径主动擦除回显。**没有发现凭据明文泄漏的确凿路径。**

**真正的问题是危险命令检测（dangercheck）**，共 4 条，其中 2 条高危：

| 编号 | 问题 | 类型 | 严重度 |
|---|---|---|---|
| S1 | 非 shell 脚本的危险内容**不被检测**（漏报） | 缺陷 | **高** |
| S2 | 非 shell 脚本里的命令字样被**误判并阻断整批作业**（误报） | 缺陷 | **高** |
| S3 | `--yes` 下放行危险命令时，**终端无任何提示** | 缺陷 | 中 |
| S4 | 危险检测只覆盖目标脚本文件本身，不含其引入的内容 | 设计边界 | 中 |
| S5 | 代填值与改密回显的保护不一致 | 设计边界 | 低 |
| S6 | `assets/` 清单脱敏依赖 IPv4 假设，失效时静默 | 设计边界 | 低 |
| S7 | 主密钥以 0644 写入 rc 文件 | 设计边界 | 记录 |

**S1 与 S2 同根同源**，都是"用 shell 语法解析一切内容"这一个原因导致的两个方向。

---

## 二、问题详情

### S1｜非 shell 脚本的危险内容不被检测

**位置**：`internal/dangercheck/parse.go`（解析层）与 `internal/ssh/command.go`（`interpreterTable`）

**机制**：`dangercheck` 用 `mvdan.cc/sh/v3/syntax`——**始终以 shell 语法解析**。而 `interpreterTable` 支持 **15 种解释器**：

```go
var interpreterTable = []interpreterInfo{
	{"bash", "-c", true, ""}, {"sh", "-c", true, ""}, {"dash", "-c", true, ""},
	{"ksh", "-c", true, ""}, {"zsh", "-c", true, ""}, {"mksh", "-c", true, ""},
	{"ash", "-c", true, ""}, {"python", "-c", false, ""}, {"luajit", "-e", false, ""},
	{"lua", "-e", false, ""}, {"node", "-e", false, ""}, {"nodejs", "-e", false, ""},
	{"perl", "-e", false, ""}, {"ruby", "-e", false, ""}, {"php", "-r", false, ""},
}
```

且**解释器可自由配置**（配置项决定脚本后缀与解释器的对应），即"什么脚本都可能"。

**实测**：

```
./SSHFleet -f nodes.csv -c 'python3 -c "import os; os.system(\"rm -rf /tmp/target\")"' --yes
```

输出：连接成功、执行成功、分类"执行成功"，**无任何危险命令警告**。目标目录**被整个删除**。

**对照**：同一路径若以裸 `rm -rf /tmp/target` 下发，会被"递归强制删除 / 高"命中并弹出警告框。

**影响**：一切非 shell 脚本（`.py` / `.pl` / `.rb` / `.js` / `.php` / `.lua`）中的危险操作**完全不被检测**。

---

### S2｜非 shell 脚本里的命令字样被误判，阻断整批作业

**位置**：同 S1

**机制**：既然用 shell 语法解析非 shell 内容，那么脚本里**恰好长成 shell 命令的文本**就会被当成真命令判定。

**实测**：一个完全无害的 python 脚本

```python
# 这个脚本本身无害：只是打印一段文字
msg = "注意：不要执行 rm -rf / 这类命令"
print(msg)
result = {"cmd": "rm -rf /tmp/example"}
print(result)
```

经 `-s` 下发（`--yes` 模式）：

```
╔════════════════════════════════════════════════════════╗
║                   ✕  发现禁止命令  ✕                   ║
╠════════════════════════════════════════════════════════╣
║    来源: 脚本: /tmp/sshfleet-test-scripts/falsepos.py  ║
║    行号: 2                                             ║
║    内容: msg = 注意：不要执行 rm -rf / 这类命令        ║
║    规则: 递归删除根目录                                ║
║    级别: 禁止                                          ║
╠════════════════════════════════════════════════════════╣
║           此命令被禁止执行，程序将立即退出！           ║
╚════════════════════════════════════════════════════════╝
```

**影响**：这不是"多报一条提醒"，而是——`forbidden` 档在 `main.go` 里是**无条件 `os.Exit(1)`**（`--yes` 也拦不住）。一个无害脚本的一句注释文字，就能让**整批 2000 台一个节点都跑不了**。

比漏报更麻烦：漏报是"没拦住"，误报是"全拦住且没有绕过手段"。

---

### S3｜`--yes` 下放行危险命令时，终端无任何提示

**位置**：`main.go` 步骤 5

```go
case len(dangerReport.Matches) > 0:
	if !in.Disinteractive {
		output.PrintDangerWarning(dangerReport, false)
	}
	confirmed, cerr := in.Confirm(common.ConfirmReq{
		Prompt:     "确定继续？",
		DefaultYes: false,
		Trace:      autoNote,
	})
	...
	if in.Disinteractive {
		dangerNote = autoNote   // ← 拼好了，但只进日志
	}
```

**实测对照**（同一条 `rm -rf` 命令）：

| 模式 | 警告框 | 询问 | 文件 |
|---|---|---|---|
| 交互 | 显示（规则：递归强制删除 / 级别：高） | `确定继续？[y/N]` | 保留 |
| `--yes` | **不显示** | **无** | 被删 |

**说明**：`--yes` 下放行 `high`/`medium`/`low` 档**是设计如此**，且判断口径合理——

- `forbidden` 档是 `switch` 的第一个分支，在 `Confirm` 之前就 `os.Exit(1)`，**`--yes` 也拦得住**
- 其 10 条规则覆盖"把机器搞死"级别（`rm -rf /`、`mkfs`、`dd of=/dev/sda`、`shutdown`、`reboot`、`init 0`、`kill -9 -1`、`systemctl mask sshd`…）
- 把 `high` 上提到 `forbidden` 会**误伤日常操作**（`rm -rf /opt/app/data` 是合法运维动作），工具将无法使用

**真正的缺陷在呈现层**：`autoNote`（"非交互模式放行危险命令（最高级别 X，分类 Y，来源行 N）"）**已经拼好，只是没往终端送**。跑完 2000 台，屏幕上只有"成功 2000"，无从知晓其中有若干台执行了 `rm -rf`。

**信息已被采集，只是没被呈现。**

---

## 三、设计边界（记录，非缺陷）

以下各项经与开发者确认，属**有意的设计取舍**或**已知边界**，记录以免日后重复排查。

### S4｜危险检测只覆盖目标脚本文件本身

`dangercheck/check.go` 只读 `a.Script` 指向的**那一个文件**：

```go
data, err := os.ReadFile(a.Script)
...
src = string(data)
```

脚本内部的 `.` / `source` 引入的其他文件、运行期 `curl | bash` 拉取的内容，**均不在检测范围内**。

这是静态检测的固有边界（不做动态追踪就无法覆盖），但需明确：**"检测过"不等于"整条执行链都看过"**。

### S5｜代填值与改密回显的保护不一致

两条路径同样是 PTY、同样开回显、同样把输出写进 `result.Output` 并落盘归档，但**只有改密路径做了脱敏**：

| 路径 | 保护 |
|---|---|
| 批量改密（`passwdsession.go`） | **有**：`scrubPasswdSecrets` 把凭据擦成 `****` |
| 交互代填（`interactive.go`） | **无**：直接 `trimOuterBlankLines(out.String())` |

**代填的设计用途是回答"选架构 / 选 deb 或 rpm"这类选项，不是承载密码**，故不构成缺陷。仅记录落差。

开发者已提出后期设想（**是否实现未定**）：增加"代填内容是否脱敏"的配置开关，默认不脱敏。

### S6｜`assets/` 清单脱敏依赖 IPv4 假设

`internal/output/archive.go`：

```go
if perr != nil || len(fields) < 4 || !common.IsIPv4Literal(fields[0]) {
	out.WriteString(line + "\n") // 表头、注释、异常行一律原样
	continue
}
```

**当前不构成风险**：`nodelist/fields.go:60` 与 `nodelist/csvread.go:42` 都强制校验 IPv4 字面量，非 IPv4 的清单行在加载阶段就被拒，故此处判断在正常流程下**恒真**。

**但失效模式是静默的**：一旦将来 IPv4 校验口径松动（例如加入主机名 / IPv6 支持），脱敏会**无声失效**——密码明文写入 `assets/` 副本，且没有任何警告。

### S7｜主密钥以 0644 写入 rc 文件

`internal/credential/rcfile.go`：

```go
return os.WriteFile(path, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
```

主密钥明文写入 `~/.bashrc`（或 `~/.zshrc`），权限 0644。实际安全性由 `~` 目录权限（通常 0755）兜底，属"恰好安全"。

**本工具面向运维人员，不对其本机操作失误设防**，故不按风险计。

**其余凭据实现核查通过**：AES-256-GCM + HKDF 标准实现、nonce 随机、日志只打 SHA-256 指纹不打密钥本身、`GuardEncrypt` 在加密前拦密钥不一致（防不可逆的错密钥加密）。

### 已核查无误、无需处理的两项

以下两处曾被评为可疑，经复核与实测确认为**实现完善**，记录以免日后重复怀疑。

**`--no-shell` 与 `--sudo` 的冲突有完整提示**

曾误判为"`--sudo` 静默失效"。实际 `internal/confirm/confirm.go` 有**三层保护**：

```go
// confirm.go:110-122 —— 专门警告 + 确认询问；--yes 下另写 SkipLine 留痕
fmt.Println("  --no-shell 是原样下发、工具不附加任何东西，所以 --sudo 不生效：")

// confirm.go:156 —— 参数屏「执行身份」行
identity = "登录用户（--no-shell 忽略 sudo）"

// confirm.go:167 —— 参数屏「解释器」行
value = "原样下发，本次不生效"
```

实测输出：

```
⚠ --sudo 与 --no-shell 同时给出
  --no-shell 是原样下发、工具不附加任何东西，所以 --sudo 不生效：
  本次将以【登录用户】身份执行，命令里不会带 sudo。
  要 root 身份，请去掉 --no-shell；要原样跑，请去掉 --sudo。

继续执行？ [Y/n]:
```

**且危险检测在 `--no-shell` 路径下照常执行**（它分析命令原文，与是否套外壳无关），不构成检测绕过。

**清单脱敏在正常流程下有效**

`internal/output/archive.go` 对第 4 列密码与第 6 列私钥口令做 `MaskSecret` 处理。其前提"第 1 列是合法 IPv4"由 `nodelist/fields.go:60` 与 `nodelist/csvread.go:42` 强制校验，正常流程下恒真。（其**静默失效模式**见 S6。）

---

## 四、建议的处理方向

按性价比排序，供参考（本次未改任何代码）：

**针对 S1 / S2（同根）**，三条路：

| 方案 | 说明 | 代价 |
|---|---|---|
| 按解释器分流 | 非 shell 解释器时跳过或换用对应解析器 | 工作量大，且要为 15 种语言各写检测逻辑 |
| 明确声明边界 | 文档写明"危险检测只对 shell 脚本有效"，非 shell 脚本自行把关 | 零成本，但等于承认该功能对非 shell 无效 |
| 限定检测范围 | 非 shell 脚本时只做关键词扫描，不套 shell 语法解析 | 中等，可消除 S2 的误判阻断，S1 的漏报仍在 |

**S2 的阻断风险高于 S1**：漏报是"没拦住"，误报是"全拦住且无法绕过"。若只做一件事，优先消除 S2 的误判。

**针对 S3**（成本最低）：`autoNote` 字符串已在 `main.go` 拼好，`--yes` 下把它往终端送一次即可——不询问、不阻断、不改 `--yes` 语义、不改档位。例如：

```
本次有 N 个节点命中危险命令（最高级别 高），已按 --yes 放行，详见日志
```

---

## 五、分析范围说明

**已覆盖**：`main.go`、`dangercheck/`（全模块）、`ssh/command.go`、`ssh/exec.go`、`ssh/interactive.go`、`ssh/matcher.go`、`ssh/passwdsession.go`、`credential/` 主要文件、`output/archive.go`、`output/output.go`、`cli/check.go` 部分、危险规则配置。

**未覆盖**：`ssh/sftp.go`、`ssh/passwdmatcher.go`、`batch/`、`nodelist/`、`verdict/`、`config/`、`confirm/`、`output/xlsx.go`、`output/report.go`。

**未实测**：文件上传 / 下载路径、批量改密路径。

---

## 六、复核方式

测试环境脚本：`build/atest-1/setup-test-env.sh`

| 项 | 命令 |
|---|---|
| 搭建环境 | `sudo bash build/atest-1/setup-test-env.sh` |
| S1 复现 | `./SSHFleet -f nodes.csv -c 'python3 -c "import os; os.system(\"rm -rf /tmp/target\")"' --yes` |
| S2 复现 | 用含 `rm -rf` 字样的无害 python 脚本，`./SSHFleet -f nodes.csv -s <脚本> --yes` |
| S3 复现 | 同一 `rm -rf` 命令分别加 / 不加 `--yes`，对比终端输出 |
| 清理环境 | `sudo bash build/atest-1/setup-test-env.sh --cleanup` |

**过程记录**：静态分析全文见 `static-report.md`，实测对照与命令见 `live-report.md`。
