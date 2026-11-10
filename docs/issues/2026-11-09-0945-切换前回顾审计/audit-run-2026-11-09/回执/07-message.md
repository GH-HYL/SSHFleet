# 07 message（回执）

**判据**：`../spec.md` 层 1（A1／A2）、层 2 的 A7（横切叶子包、终端只出 `console`、拼句只出 `message`）与 `ADR-0032` 第四条及其 13 条更新记录。只认分给本轴的那几条；A4／A5／A6／A8／A9 归 01／08／09，不替它们判。

**只读**：未改 `modules/` 下任何文件。临时扫描器落 `build/ax07_scan_han.py`（只读源码），未跑构建、未跑测试、未上真机。

**证据口径**：本轴主体是**计数类**。`file:line` ＋ 一条可重跑的命令 ＋ 结果表。扫描器只做一件事：去掉 `//` 与 `/* */` 注释后，取每行里含汉字（`\u4e00-\u9fff` 等区段，含全角标点）的字符串字面量，按包与文件计数。**注释里的中文不计**。行内多处字面量按处数计。

复算：

```bash
python build/ax07_scan_han.py modules/SSHFleet2_Go
```

---

## 一、全工程分布（分母的第一项）

18 个包、**152 个生产 `.go` 文件**全扫（塔尖 1 ＋ `internal/` 十七包），含汉字字面量共 **964 条**。

| 包 | 生产文件数 | 含汉字字面量 | 其中带 `%` 动词 | 纯文本 |
| --- | ---: | ---: | ---: | ---: |
| `internal/message` | 25 | **488** | 75 | 413 |
| `internal/parse` | 17 | 212 | 64 | 148 |
| `internal/guard` | 15 | 139 | 29 | 110 |
| `internal/execute` | 22 | 82 | 31 | 51 |
| `internal/credential` | 10 | 16 | 7 | 9 |
| `internal/report` | 4 | 10 | 9 | 1 |
| `internal/archive` | 8 | 5 | 5 | 0 |
| `internal/keytool` | 2 | 4 | 0 | 4 |
| `internal/log` | 2 | 4 | 0 | 4 |
| `internal/gate` | 5 | 2 | 0 | 2 |
| `internal/mask` | 5 | 2 | 2 | 0 |
| `main.go` | 1 | 0 | 0 | 0 |
| `internal/ask` | 2 | 0 | 0 | 0 |
| `internal/console` | 12 | 0 | 0 | 0 |
| `internal/contract` | 17 | 0 | 0 | 0 |
| `internal/exitcode` | 2 | 0 | 0 | 0 |
| `internal/interrupt` | 1 | 0 | 0 | 0 |
| `internal/settle` | 2 | 0 | 0 | 0 |
| **合计** | **152** | **964** | 222 | 742 |

**基准答案（`message` ＋ `credential/words.go` 两处）核对结果**：

- `message`：488 条 / 25 个文件。
- `credential/words.go`：13 条（另有 `credential.go:112`、`persist_windows.go:24`、`rcfile.go:14` 三条同包不在这个口子里，性质见 §二）。
- **第三处及以后：463 条 / 48 个文件 / 11 个包**。

`contract`、`ask`、`console`、`exitcode`、`interrupt`、`settle`、塔尖：**0 条**——这几个包一个字都不成形，与 ADR-0032 第三条「契约只放类型与常量」、结算站不写字对得上。

---

## 二、第三处候选（逐包：它拼的是什么、本该由谁拼）

> 下面只给位置、条数与「拼的是什么」。**逐站是否违规由 05／06／08 判，本轴不替它们下结论**；契约格子的合规性开发者未裁（§1.8），§四只登记事实。

### 2.1 `parse`：212 条 / 17 个文件

| 文件 | 条数 | 拼的是什么 | 去路 |
| --- | ---: | --- | --- |
| `config.go` | 60 | 配置起不来的「问题／原因／提示」整句（`config.go:147-195`、`339-484`、`530-564`）：`message.ProblemHint` 的两个入参**两半都由站里写**，`message` 只贴「原因：／提示：」两个标签与换行 | 经 `parse/run.go:80` 进 `contract.CannotStart.Body` → 塔尖 → `message.Fatal` 上屏 |
| `tables.go` | 41 | 四张判据表（危险规则／错误关键词／改密／判据）校验失败的整句（`tables.go:96-120`、`150-187`、`215-359`） | 同上 |
| `options.go` | **31** | **帮助屏的全部说明文字**：5 个组标题 ＋ 20 条选项说明 ＋ 4 个标记（`options.go:57-98`）。`message/help.go` 只给列宽、缩进、折行、用法两行与示例八条 | `message.HelpScreen` 排版 → `console.Print` |
| `nodes.go` | 21 | 清单逐行补全的错误（`nodes.go:60-131`、`182-186`）、凭据预检查的行级句子（`:211-231`）、端口／用户名兜底（`:274`、`:302`） | 同上，或作为台账行上屏 |
| `material.go` | 20 | `-s`／`-p`／`-f` 材料自检的十组「问题／提示」（`material.go:30-97`） | 同上 |
| `nodelist.go` | 13 | 清单读入错误的整句（`:78-115`）、CSV 行格式错误（`:157-163`） | 同上 |
| `collect.go` | 11 | 上传采集与软链接过滤的整句（`:39-100`、`:163`、`:181-183`） | 同上 |
| `toml.go` | 6 | TOML 词法错误（`:41-56`）＋ 示例标记（`:94`） | 同上 |
| `argv.go` | 3 | 顿号连接符（`:228`、`:468`、`:471`） | 拼进 `contract.Problem.Detail` 与拒绝句 |
| `runmeta.go` | 3 | 封运行上下文失败的「封装运行上下文：…」 | 同上 |
| `run.go` | 2 | `cannotStart` 的半句「加载配置文件失败（」「）」 | `message.Shell` 套壳 |
| `passwd.go` | 1 | 顿号 | 拼改密值问题串 |

**按 A7（拼句只出 `message`）本该由 `message` 拼的字：212 条中至少 208 条是整句或半句**（`argv.go`／`passwd.go` 那 4 个顿号不算句子）。其中 31 条在帮助屏，176 条在「起不来」屏，5 条是拒绝句的零件。

### 2.2 `execute`：82 条 / 22 个文件

| 文件 | 条数 | 拼的是什么 | 去路 |
| --- | ---: | --- | --- |
| `log_session.go` | 41 | 执行期日志的整段说明：执行参数、开始执行任务、代填表、四种「交给 SSH 执行」段、身份与处理方式（`:37-178`） | 工具日志（`lg.Info` 族），不上屏 |
| `log_node.go` | 17 | 执行期日志的逐节点行：连接、完成、失败、错误详情、分类、输出明细（`:44-175`） | 同上 |
| `transfer_view.go` | 5 | 传输进度面板：「进度」「节点进度」「成功 %d」「失败 %d」「共 %d 个文件」 | **`console.Writer()` 那条流**（§三） |
| `progress.go` | 4 | 命令／脚本进度行：「总进度」「已完成 %d/%d」「成功 %d」「失败 %d」 | 同上 |
| `client.go` | 4 | 认证方式标签「密钥」「密码」「未配置」＋ 一条起不来句 | 台账／起不来 |
| `run_download.go` | 4 | 结果明细行「已跳过（符号链接）」「下载失败 - 」 | `contract.NodeResult.Output` → 上屏 |
| `run_upload.go` | 4 | 同上「上传失败 - 」 | 同上 |
| `reporter.go` | 2 | 「建终端输出文件失败」「建执行期日志失败」 | `contract.CannotStart.Body`（`execute/run.go:49` 直接 `err.Error()`） |
| `run_passwd.go` | 1 | 一个冒号 | 拼进明细 |

两个来源：**日志 58 条**（`log_session` ＋ `log_node`）与**面板 9 条**，其余 15 条进结果明细／起不来。

### 2.3 其余包

| 包 | 条数 | 拼的是什么 | 本该由谁拼 |
| --- | ---: | --- | --- |
| `guard` | 139 | 检查器自己的诊断：七类九项每条的三段（现象／为什么／该走）＋ 台账表头 ＋ 红线名单报错。**不在产品五模式路径上**，是开发工具的产出 | 02 已收账，本轴只列数 |
| `credential` | 16 | `words.go` 13 条（**已裁的唯一口子**，记录 13）；另 3 条：`credential.go:112` 的 `"%w：%s"` 拼接、`persist_windows.go:24` 的注册表路径、`rcfile.go:14` 写进 rc 文件的注释行 | 前 13 条只当事实；后 3 条里 `credential.go:112` 是一条格式化句，落点属 08 |
| `report` | 10 | 写盘失败的 `fmt.Errorf`（`report.go:45-55`）＋ 解运行上下文的六条（`runmeta.go:32-60`） | 其中四条与 `parse/runmeta.go` 同句（§七 M1） |
| `archive` | 5 | 建目录／备份／清理／读历史失败的 `fmt.Errorf` | `message`（若上屏） |
| `keytool` | 4 | 方向名词「明文」「密文」（`:110`、`:130`），作为 `message.KeyConvertDone` 的入参 | `message` |
| `log` | 4 | 工具日志头两行：「SSHFleet工具开始执行」「工作路径：」「日志目录：」「，工具日志文件名：」 | `message`（它是日志，不是终端；按 A7 的字面口径仍算） |
| `gate` | 2 | 名词「上传目录」「远程路径」（`run.go:174`、`:187`），拼进 `message.RefuseRemotePathNotAbsolute` | `message` |
| `mask` | 2 | 清单读写失败的 `fmt.Errorf` | `message` |

`gate` 那两条值得单独看一眼：闸门自己不写整句，但它给 `message` 递了句子的**名词**（`RefuseRemotePathNotAbsolute(flag, what, value)` 的 `what`），"本机合法却省事"最小的一档。合规归 08。

---

## 三、`console` 直写路径

| 通道 | 调用点 | 收什么 | 字源约束 |
| --- | ---: | --- | --- |
| `console.Print` | **17** 处（`ask` 6、`report` 4、塔尖 7） | `message.Line` | **编译期硬闸**：`Line` 字段不导出，包外造不出（全工程 `Line{s:` 0 处；`message.Line{}` 38 处全是"没有话"的空值） |
| `console.Writer` | **1** 处（`execute/run.go:44` → `newScreen`） | `io.Writer`（`os.Stdout`） | **无**：拿到流之后写什么由执行站自己定，进度帧 9 条中文字面量（§2.2） |
| `os.Stdout`／`os.Stderr`／`fmt.Print`／`println` | **0** 处（`console` 包外，`guard/testdata` 不计） | — | 类一在守 |
| `Line.Text()` 的包外读点 | `execute/reporter.go:65,68`（上屏流 ＋ `terminal-output.txt`）、塔尖 6 处（`main.go:87,97,106,107,145,154`，日志与留痕）、`gate/confirm.go:23,27,31,37,54` ＋ `gate/run.go:46`（只判空串） | 同一句的两个形态 | 与 `line.go:28` 注释「只有 console 该调它」不符，但用途都是「把同一句话送到非终端去」，不是另造一句 |

**结论（本轴第 2 问）**：绕过 `message` 直接往屏幕上写字的路径**只有 1 条**——`console.Writer` 那条流。它被点名授权（2026-11-02 为进度界面开的口子，`console/writer.go:6-15`），代价是那一段屏上字完全不受字源约束。01 已就"能不能通"报过（S14），本轴只补**这一段的字有多少**：2 个文件、9 条字面量，另有 `message.ActionName`／`FormatSpeed`／`Bytes` 三个标签从 `message` 取。

---

## 四、契约里装字的格子（只登记，不判合规）

开发者未裁（§1.8「契约能不能装拼好的句子」）。逐格：

| 格 | 定义处 | 谁填 | 填原句还是片段 |
| --- | --- | --- | --- |
| `CannotStart.Body` | `contract/cannot_start.go:23` | `parse/run.go:80`（`message.Shell(head, body)` 的**产物**，`head`／`body` 由站里给）；`execute/run.go:49`（`err.Error()`，句子出自 `execute/reporter.go:42,47`）；`keytool/run.go:193`（`message.KeyConvert*` 的整句，或 `err.Error()`） | **原句**（带换行、已分行、无颜色）。塔尖 `main.go:64` 原样交给 `message.Fatal`，只在最外层补 `[ERROR]` 前缀与「日志：」一行 |
| `Problem.Detail` | `contract/plan.go:132` | `parse/argv.go:167,178,187` 填 `message.SubjectConcurrency/Timeout/Connect`（**名词**）；`argv.go:228` 填 `strings.Join(modes,"、")`（模式名）；`parse/downlink.go:40` 填解释器名。读点：`gate/run.go:228,288` | **片段**（名词／枚举），不是句子；注释自陈「一句事实说明」 |
| `Ledger.Verify` | `contract/ledger.go:38` | `settle/settle.go:58,62` 填 `message.VerifyPassed`／`VerifyFailed`（「通过」「异常」） | **一个词**，闭集；`message/stats.go:67` 拿它当分支判据（同一个词既上屏又当状态码） |
| `Answer.AbortLine` | `contract/answer.go:31` | `execute/run_answer.go:70`（`outcome.abortLine`） | **远端提示原文**（数据，不是工具的字） |

不算装字的格（一并说明，免得读的人以为漏了）：`DangerMatch.Content/Example/Reason`（配置原文）、`KeyFacts.EnvPrint/PersistPrint`（八位指纹）、`NodeResult.ErrorText/Output`（远端原文）、`Notice{Kind,Rows}`（只有种类与行号，句子在 `message/notice.go:17` 现拼）。

**一处与提示词原文对不上**（进「顺带」D1）：提示词把 `ResultRow.Detail` 算作契约格，实际 `ResultRow` 定义在 `message/result_rows.go:22`，不在 `contract`；它装的是 `message` 自己算出来的东西（`res.ErrorText`、输出逐行），不是别站塞进来的字。

---

## 五、宽度实现

| 实现 | 位置 | 量的是什么 |
| --- | --- | --- |
| 自研（唯一一处） | `message/width.go:22-266`（EAW 表 ＋ `runeWidth`／`displayWidth`／`trimToWidth`／`wrapByWidth`／`padTo`，导出 `DisplayWidth`） | 字符显示宽度、截断、折行、补齐。**全工程只此一份**：`parse/tables.go:158,161`、`report/xlsx.go:244,272` 都调 `message.DisplayWidth` |
| 终端列数查询 | `console/width_linux.go:19`／`width_windows.go:16`／`width_other.go:6` | 终端有多少列（ioctl／`GetConsoleScreenBufferInfo`）。**不是字符宽度**，不与上一份重复 |
| 第三方（进了二进制） | `clipperhouse/displaywidth`、`mattn/go-runewidth`、`rivo/uniseg`（经 `lipgloss`→`charmbracelet/x/ansi`、`bubbles/progress`） | 进度面板排版时量字符串宽度。证据：`go list -deps ./internal/execute` 里三条都在 |

所以「宽度实现几份」的准确答法是：**字符宽度算法 3 份在进程里**（自研 1 ＋ 第三方 2，`uniseg` 是分词不算宽度），**但只有自研那份被 `message`／`parse`／`report` 用**；第三方那两份只服务执行站的进度面板（§三那条流）。两份第三方与 `message.DisplayWidth` 是否逐字符同结果，**本轴未验证**（见未覆盖面）。

另有一处包内不一致：`message/confirm.go:45` 的边框用 `len([]rune(confirmTitle))+10` 算，全包其余排版都用 `DisplayWidth`。今天 `confirmTitle` 是纯 ASCII，两种量法同值，所以现在看不出差别（§七 M7）。

---

## 六、25 个文件按用途分类

| 类 | 文件（行数） | 小计 |
| --- | --- | --- |
| 载体与地基 | `line.go`(39)、`width.go`(266)、`doc.go`(6) | 3 |
| 整屏 | `screens.go`(119)、`help.go`(116)、`confirm.go`(275)、`confirm_rows.go`(205)、`danger.go`(138)、`keytool.go`(338) | 6 |
| 行与块 | `result.go`(153)、`result_rows.go`(73)、`stats.go`(371)、`interrupt.go`(13)、`nodelist.go`(25)、`notice.go`(86)、`link_warn.go`(23)、`xlsx_warn.go`(12) | 8 |
| 标签与词表 | `labels.go`(36)、`category.go`(43)、`modename.go`(141) | 3 |
| 某一路的话 | `answer.go`(118)、`passwd.go`(94)、`refuse.go`(250)、`step_note.go`(61) | 4 |
| 系统原因 | `system.go`(26) | 1 |
| 合计 | | **25** |

**「一处成形」还是「一处堆放」**：主体是**成形**——模式名字表（`modename.go` 五列）、分类名（`category.go`）、结果行（`result.go` 一份喂屏幕／`terminal-output.txt`／日志三处）、统计块与 `report.txt`（`stats.go`）都是"一处出字、多处消费"，且消费方真的都来取（`grep` 到 `ActionName`／`SuccessCategory`／`CategoryOfExitCode`／`NodeTag` 的跨包调用点）。

但有**一角是堆放**：同一种标签在包内各写一遍（§七 M6，7 组 22 处），以及 `stats.go` 一个文件装了五件事（§七 M9）。判据是「同一类字有没有在这一处内部又各写了一份」，不是行数——所以 3,027 行本身不扣分。

---

## 七、发现

### M1【硬伤】同一句整句在两个包里各写一遍：8 组、25 处，`message` 的"唯一改字处"对这几句不成立

- **位置与证据**（腐蚀类：无命令可跑，给 `file:line` 与可复算计数）：

  | # | 句子 | 两处（或多处）位置 | 处数 |
  | --- | --- | --- | ---: |
  | 1 | 「请检查文件是否存在、当前用户有没有读权限」 | `message/answer.go:18`；`parse/material.go:38`、`:87`；`parse/nodelist.go:80` | 4 |
  | 2 | 「值里要用引号时，把里面的引号写成两个」 | `message/answer.go:49`；`parse/nodelist.go:163` | 2 |
  | 3 | 「不存在（打开加密时这里填的是凭据文件的绝对路径，不加密时才是密码本身）」 | `message/passwd.go:73`；`parse/nodes.go:227` | 2 |
  | 4 | 「无法读取」 | `message/passwd.go:75`；`parse/nodes.go:231` | 2 |
  | 5 | 「必须是绝对路径」（拼在「%s 参数指定的%s必须是绝对路径」里） | `message/refuse.go:141`；`parse/config.go:552`、`:564` | 3 |
  | 6 | 「运行上下文：建不出密码器：%w」／「…建不出 GCM：%w」 | `parse/runmeta.go:100`、`:104`；`report/runmeta.go:56`、`:60` | 4 |
  | 7 | 「-f 参数指定的 CSV 文件读不到：%s」／「…为空：%s」／「一行写一台机器，只写 IP 就能跑（端口、用户名、密码走配置）」 | `parse/material.go:85`、`:91`、`:92`；`parse/nodelist.go:78`、`:84`、`:85` | 6 |
  | 8 | 「分类: 」 | `message/result.go:72`；`execute/log_node.go:102` | 2 |

  另有 3 组是**半句／名词**重复、不算整句：`message/notice.go:83` 与 `parse/collect.go:183` 的「 等 %d 个」（`message` 注释自陈"两处各拼一遍"）；`message/confirm.go:83,87` 与 `gate/run.go:187` 的「远程路径」；`message/confirm.go:141` 与 `execute/log_session.go:126` 的「登录用户」。

- **后果**：第 1、2、3、4、5 组都是**同一句使用者会读到的话**有两个写入点，改一处漏一处就出现两种说法——这正是 `labels.go:4-8` 自己写明要避免的形态（那里把「当前值：」「取值非法：」抽成常量，理由就是"同一句话有好几个写入点，改一个说法就得逐处找"）。第 6 组更明显：加解密失败的同一句在解析站与报告站各写一遍。
- **判据**：ADR-0032 第四条「`message` 唯一拼句处、唯一改字处」（明示锚）。**本条不依赖契约格子那条未裁项**——这几组是"两边都在拼同一句"，不是"纸能不能装句子"。
- **伤**：腐蚀架构（同一句话两个产出，已经开始各改各的：第 3、4 组 `message` 那份是 `PasswdProblemLabel` 的六个码之一，`parse` 那份是 `credLabel` 的三个分支，两边的分类口径已经不同——`message` 有 6 类、`parse` 只认 3 类）。
- **归口**：04 的"重复密度／同一句文案"与本节重叠，判读以 04 的重复形态表为准；本轴只声明它在"字是否只在一处成形"这条上的意义。

### M2【判断题】帮助屏的字分两处：`parse` 31 条说明 vs `message` 9 条版式

- **位置与证据**：`parse/options.go:57-98` 写全部组标题与选项说明（31 条，含 4 个 `[默认: …]`／`[当前配置: …]` 标记）；`message/help.go:43-105` 只写用法两行、示例八条、标题／版本／结尾三行（9 条）与折行补齐。
- **两读都成立**：ADR-0032 第五条写「帮助里「有哪些选项、各是什么」由解析站给（与选项定义同源，分开就有两个来源），**话由 `message` 拼**」——"各是什么"可以读成"这些说明文字就是事实"，也可以读成"事实是选项名与默认值，说明的措辞归 `message`"。`message/help.go:5` 自己写的是「字只在这一处成形」，与 `parse/options.go` 的 31 条直接抵触。
- **代价**：改一句帮助文案要去 `parse`；而"帮助排版只此一处"的测试（`message/help_test.go`）钉的是列宽与折行，钉不到措辞来源。
- **判据**：ADR-0032 第四条／第五条。措辞归谁，06 与本轴的读数会不同，**建议由 06 一并判**，本轴只给计数。
- **伤**：只是架构取舍（若判"说明文字属事实"）／腐蚀架构（若判"措辞也是话"）。

### M3【判断题】契约装字四格：整句一格（`Body`）＋远端原文一格（`AbortLine`）＋片段一格（`Detail`）＋一个词一格（`Verify`）

- 事实见 §四，不重复。**开发者未裁**，本节不写合规结论，也不拿它当别的判断的落点。
- 唯一可说的结构事实：`CannotStart.Body` 的**三个填充点有两种形态**——`parse`／`keytool` 填的是 `message` 产出的整句（外壳已套好），`execute` 填的是 `err.Error()`（句子出自执行站自己）。同格两形态，改口径时要知道有两条路。
- **伤**：待定。

### M4【判断题】进度面板那一段屏上字完全不受字源约束（9 条）

- **位置与证据**：`execute/progress.go:116-120`、`execute/transfer_view.go:232-255`，共 9 条中文字面量（其中「成功 %d」「失败 %d」两处各写了两遍：`progress.go:119,120` 与 `transfer_view.go:243,244`）。
- **判据**：A7「终端只出 `console`、拼句只出 `message`」。流的那一半已授权（2026-11-02，`console/writer.go:6-15`），**字的那一半没有写在授权里**——授权原文只说"借 writer 的人只许一个"，没说"这一段可以自己写字"。
- **代价与边界**：这一段是"面板的一部分"（代码注释的说法），要搬进 `message` 就得给 `message` 加进度面板的版式函数（`message` 现在已经有 `danger.go` 的框、`confirm.go` 的表，先例是有的）。搬不搬是 05 的判断，本轴只指出"授权覆盖了流、没覆盖字"。
- **伤**：只是架构取舍（授权明确）／腐蚀架构（授权没覆盖到字）。

### M5【判断题】字符宽度算法在进程里有 3 份，只有 1 份是全工程共用的

- 事实见 §五。自研那份是唯一算法且被三处调用；第三方两份只服务进度面板。
- **代价**：面板上中文标签的宽度由 lipgloss／bubbles 量，屏幕上其余地方由 `message.DisplayWidth` 量——两套 EAW 表、两套零宽字符判定。今天面板不用对齐（固定宽度条 ＋ 定长分隔线），所以看不出差别；一旦面板要跟别处对齐，就是第二处口径。
- **判据**：ADR-0032 更新记录 2026-11-01「取消 `display` 包，宽度与折行并进 `message`……理由是「字只在一处成形」」。那条记录针对的是自研实现；第三方依赖是"进度界面"这个授权带进来的副作用，**有没有被评估过，从代码里看不出来**。
- **伤**：只是架构取舍。

### M6【判断题】`message` 内部：7 组标签各写一遍、22 处，`tagLabel` 只统一了方括号

- **位置与证据**：

  | 组 | 处数 | 位置 |
  | --- | ---: | --- |
  | 「警告」 | 3 | `keytool.go:87`、`link_warn.go:13`、`link_warn.go:20` |
  | 「ERROR」 | 2 | `screens.go:50`、`xlsx_warn.go:10` |
  | 「提示：」 | 7 | `screens.go:18`（`hintLabel` 常量）、`keytool.go:41`、`link_warn.go:21`、`refuse.go:238`、`refuse.go:248`、`stats.go:120`、`stats.go:203` |
  | 「本地路径」 | 2 | `confirm.go:82`、`confirm.go:88` |
  | 「远程路径」 | 2 | `confirm.go:83`、`confirm.go:87` |
  | 「成功」／「失败」 | 4 | `result.go:80,82,89,91`（`connStatus` 与 `statusLine` 各写一遍） |
  | 「下一步：」 | 2 | `keytool.go:58`、`keytool.go:64` |

- **后果**：`tagLabel`（`labels.go:33`）只统一了配色与方括号，**标签词本身仍是自由字符串**——`"警告"` 写错一个字，`tagLabel` 看不出来。同理 `hintLabel` 做成了常量，但另外 6 处写「提示：」的地方都不用它（其中 `stats.go:120,203` 是"屏幕行标签"、`refuse.go:238,248` 是"句子里的提示段"，版式不同，未必该共用一个常量）。
- **判据**：`labels.go:4-8` 与 `category.go:3-6` 自陈的规矩（"同一句话有好几个写入点，改一个说法就得逐处找"）。这是包内自设的口径，不是外部最佳实践。
- **代价**：改一个标签词要扫本包；`warning`／`提示` 这类词在屏上的统一性靠人守。
- **伤**：只是架构取舍（数量小、都在一个包里，改的时候一屏能看全）。

### M7【判断题】`message/confirm.go:45` 的边框宽度用 `len([]rune())`，与全包 `DisplayWidth` 是两套量法

- **位置与证据**：`border := strings.Repeat("═", len([]rune(confirmTitle))+10)`（`confirm.go:45`）；`confirmTitle` 是纯 ASCII（`confirm.go:29`），所以今天两种量法同值。包内其余（`help.go:76`、`infoTable`、`danger.go` 的 `center`／`padTo`）全用 `displayWidth`。
- **后果**：一旦标题里出现一个全角字符（改文案时最常见的动作），边框就短两列，框线错位——`danger.go:1-6` 记的正是"两个感叹号没对齐"那次真机反馈。
- **判据**：ADR-0032 更新记录 2026-11-01「宽度……并进 `message`」的意图是"量法只此一处"；同在 `message` 里出现第二套量法，属未爆的隐患。
- **伤**：只是架构取舍（当前无行为差异）。

### M8【判断题】解释器名的归一规则两处实现：`parse/downlink.go:292` 与 `message/confirm.go:157-170`

- **位置与证据**：`parse/downlink.go:292`：`strings.TrimRight(strings.ToLower(path.Base(fields[0])), "0123456789.")`；`message/confirm.go:157-170` 的 `isBashInterpreter` 做同一件事（取第一个词、去目录、转小写、去尾部版本号），注释自陈"归一规则与解析站那张解释器表同源"。`modename.go:126-135` 还存着第三份"解释器名单"（只给报错列名用，注释说"两处不许漂：解析站有一条用例按集合比对"）。
- **后果**：`isBashInterpreter` 只决定确认屏要不要显示「解释器」这一行。两边漂了不会报错，只会让"配的是 bash 却多显示一行"或反过来——**静默的显示差异**，`message` 的测试钉不到 `parse` 的表。
- **判据**：ADR-0032 第四条（拼句包不该带解析规则）；这是"另造一套"里最轻的一档（一个显示门槛）。
- **伤**：腐蚀架构（同一规则两个实现，漂了没人报）。

### M9【判断题】`stats.go` 一个文件装五件事，一句话说不清

- **位置与证据**：`stats.go`（371 行）里同时有：屏幕统计块 `StatisticsLine`（`:57-130`）、`report.txt` 全文 `ReportText`（`:137-223`）、argv 还原命令行 `DisplayCommand`／`quoteForShell`（`:231-266`，一套 shell 引号规则）、失败分类提示 `categoryTipLines`／`exitCodeHintLine`（`:268-330`）、日志收尾行 `ExecSummaryLog`（`:334-338`）、速度写法 `FormatSpeed`（`:341-350`）。
- **判据**：`docs/个人开发规范` 三.2「模块内各文件的用途应能被一句话说清」。这个文件的一句话只能是"统计与报告的字，外加两个格式化"，已经是两件事。更突出的 `DisplayCommand`：它是一条**给 shell 的引号规则**，与"字"无关（注释也承认它与执行站拼下发行"不是同一份"）。
- **代价**：改 `report.txt` 版式与改"速度写法"要进同一个文件；`quoteForShell` 的正确性靠自己的测试，与统计块无任何关系。
- **伤**：只是架构取舍。

### M10【判断题】执行期日志的字也是两个来源：`execute` 58 条 vs `message` 若干条

- **位置与证据**：`execute/log_session.go`（41）＋ `log_node.go`（17）自拼整段日志；`message` 里也有日志用字：`interrupt.go:12`（`InterruptLog`）、`step_note.go:53`（`StepNote`）、`stats.go:334`（`ExecSummaryLog`）、`screens.go:120-134`（`TraceDanger`／`TraceForbidden`）、`confirm.go:246`（`ConfirmSkippedTrace`）、`keytool.go:221`（`KeyModeEntered`）。
- **后果**：同一条日志流里，有的行由 `message` 出、有的行由执行站出；`Line.Plain()`（`line.go:31`）这条"日志形态"的约定只对 `message` 那半边成立。
- **判据**：A7「拼句只出 `message`」。日志不在屏幕上，但"字"是同一套——ADR-0032 第四条没有把日志排除在外。
- **伤**：只是架构取舍（日志不给使用者看）／腐蚀架构（若字的口径要统一）。逐条是否算违规归 05。

---

## 八、分母

| 项 | 数 |
| --- | --- |
| 扫了几个包 | **18**（`internal/` 十七 ＋ 塔尖）；152 个生产 `.go` 文件全扫，除 `_test.go` 与 `testdata/` |
| 中文成形点 | **964 条**字面量：`message` 488、`credential/words.go` 13、**其余 463（48 文件／11 包）** |
| 逐包分布 | 见 §一（11 个包非零，7 个包为零） |
| `console` 直写路径 | `Print` 17 个调用点（全收 `message.Line`）；`Writer` **1** 条绕开字源的路径；`os.Stdout`／`os.Stderr`／`fmt.Print`／`println` 包外 **0** |
| 契约装字格 | **4 格**（`CannotStart.Body`、`Problem.Detail`、`Ledger.Verify`、`Answer.AbortLine`）＋ 4 类数据格（`DangerMatch` 三格、`KeyFacts` 指纹／路径、`NodeResult` 原文、`Notice` 种类） |
| 宽度实现 | **3 份字符宽度算法**在进程里（自研 1 ＋ 第三方 2）；终端列数查询另一件事（3 个平台文件）；包内第二套量法 1 处（M7） |
| 25 个文件分类 | **6 类**（载体 3／整屏 6／行与块 8／标签词表 3／某一路的话 4／系统原因 1） |
| `message` 导出面 | 导出函数 **149**、导出方法 **3**（`Line.Text/Plain/Stderr`）、导出的业务常量 **16**（`CategoryPasswdExpired`／`CategoryUnknown`／`ConfigShownPath`／`HeadParse`／`HeadCompliance`／`HeadDangerRules`／`SubjectConcurrency`／`SubjectTimeout`／`SubjectConnect`／`NoNodeParsed`／`ManifestGoneHint`／`VerifyPassed`／`VerifyFailed`／`RowEventError`／`RowEventOutput`／`ConfigNoEncrypt`） |
| 报了几条 | **硬伤 1（M1）＋ 判断题 9（M2–M10）＝ 10 条** |

---

## 九、未覆盖面

1. **`parse`／`execute` 的文件正文没有全读**：本轴只读了与"字在哪成形"直接相关的 24 个文件（`parse` 的 `options`／`config`／`nodes`／`nodelist`／`material`／`collect`／`run`／`tables` 片段，`execute` 的 `progress`／`transfer_view`／`log_session`／`log_node`／`reporter`／`run` 片段）。两个包共 39 个生产文件，剩下的（`argv`／`script`／`downlink`／`passwd`／`client`／`transfer`／`run_*` 等）只看了扫描器的行级命中，**没有读上下文判断"这一句是给谁看的"**。所以 §2.1／§2.2 的"去路"一列有几行是按调用关系推的，不是逐句读出来的。
2. **`guard` 的 139 条只当分布**，一条都没读（02 已收账，且它是开发工具不是产品路径）。它算不算"第三处"取决于"全工程"是否包含工具，本轴按"包含"计数、按"不算"判读。
3. **第三方宽度实现与 `message.DisplayWidth` 的逐字符一致性未验证**：没有写对比程序（本轮不跑构建）。§五只给"存在"与"谁在用"，不给"结果一样／不一样"。
4. **`contract` 装字格合规性未判**（开发者未裁），也没有统计"经过 `CannotStart.Body` 上屏的字占屏幕正文多少比例"——那需要逐屏还原，超出本轮文件域。
5. **`message` 的 14 个 `_test.go` 未读**（11,269 行测试代码里属本包的部分）。本轴只用了生产代码，测试能否钉住 §七各条（尤其 M1 的重复句、M6 的标签）未验证。
6. **老工程对照只抽了一处**：`modules/SSHFleet_Go/internal/output/` 只对照了「错误详情: 」与「错误详情：」两种冒号（确认是**老版就有的差别**，`terminal.go:105` 半角、`reporter.go:281` 全角，不是新工程漂的，故未报）。§六第 6 问"有没有把老版某句话原样搬来又按老版特判"因此是**抽查结论**：`message` 里没搜到任何"只有老版才需要"的分支（`grep '老版才\|只有老版\|兼容'` 零命中），但没有做逐句 diff。
7. **§二没做"每条第三处候选都判一次 A7"**：按提示词要求，逐站合规归 05／06／08；本轴只给"拼的是什么"与"按 A7 该由谁拼"。因此这 463 条里"有几条真违规"这个数**本回执没有**。

---

## 顺带

- **D1**：提示词 §5.3 把 `ResultRow.Detail` 算成"契约里装字的格子"。实际 `ResultRow` 在 `message/result_rows.go:22`，不在 `contract`；契约里的字格是 `CannotStart.Body`、`Problem.Detail`、`Ledger.Verify`、`Answer.AbortLine` 四个。不影响任务，只影响读的人去找文件。
- **D2**：`message/doc.go:5` 与 `message/help.go:5` 都写"字只在一处成形"。按本回执 §一，全工程有 463 条字在 `message` 之外成形（其中 `parse` 212、`execute` 82）。代码注释把"出口唯一"（真）与"成形唯一"（不真）写成了同一件事——这是注释与实现不符，不是新工程行为问题。
- **D3**：`message/width.go:15` 写「新工程零第三方依赖，表自己带」，而 `go.mod` 有 7 个直接第三方依赖（`bubbles`／`bubbletea`／`lipgloss`／`sftp`／`excelize`／`x/crypto`／`x/sys`），其中 `x/crypto`、`x/sys` 被 `internal/` 直接 import（`execute/client.go:21`、`archive/link_windows.go:10`）。那句话的语境是"宽度表自己算、不借 `x/text`"，就宽度而言是真的；写成"新工程零第三方"不是事实。锚与规格里没有这句，是注释自己加的，属文档陈旧。（`console/width_windows.go:27` 的「不引第三方」是包内依赖约束，是真的，不算这一条。）
