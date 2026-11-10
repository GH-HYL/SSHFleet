# 05 包内 `execute`（回执）

**文件域**：`modules/SSHFleet2_Go/internal/execute/` 全部生产文件 22 个、4,087 行（`wc -l` 复核，与基线一致）；19 个 `_test.go`（2,766 行）只作旁证，不进分母。对照物：`modules/SSHFleet_Go/internal/{ssh,batch,output}/` 的对应片段。只读；`build/` 下没落新文件（全部静态读）。契约只读了 `contract/{plan,result,ledger,runmeta}.go` 里用到的格子；`message` 只读了导出清单与 `labels/stats/result/confirm_rows` 四份，用来判「本该用谁」。

**读法声明**：本条只判包内四种形态与长抽屉，不判 import 方向（01 已收账）、不判红线测试（02）、不判契约格装不装字（未裁，§1.8）。`execute` 里凡是「五站单向对不对」的判断一律没写。轮一的 15 个代码侧旧名残留、六组兄弟都没重开。

---

## 一、另造一套：本该调横切包，`execute` 自己实现了一份

**扫了 12 个能力面**（`console.Writer`／`console.Print`／`console.IsTerminal`／`mask.Scrub`／`log.InitExec`＋五级方法／`message.ResultLine`／`message.Bytes`／`message` 的 16 个导出函数／`message.ExecSummaryLog`／`archive.Paths`／`contract` 类型／宽度折行），约 40 个调用点。用对的 9 个能力面不记；本节报 2 条（F1、F7）。另两处「本该在 message、message 里没有对应物」的（日志句、面板标签）放在第三节按拼话计数，不在这里重复计。

### F1【硬伤】同一句日志字两处成形：「分类: 」逐字重复，「错误详情」已经漂成全角/半角

- **位置与证据**：
  - `internal/message/result.go:72` —— `func categoryLine(res contract.NodeResult) string { return "分类: " + res.Category }`，注释（`:71`）写「终端那一段与表格那一行共用，**改说法只改这里**」。
  - `internal/execute/log_node.go:102` —— `lg.Info(ip + "分类: " + res.Category)`。字面量逐字相同，成形点是第二处。
  - `internal/message/result.go:55` —— `"错误详情: "`（冒号 `0x3A`，半角）；`internal/execute/log_node.go:87` —— `"错误详情："`（`EF BC 9A`，全角）。同一台机器的一次运行里，屏幕/`terminal-output.txt` 印半角、`SSHFleetExec.log` 印全角——**漂移已经发生，不是将来风险**（字节已用 `od -c` 核过）。
  - 计数：2 组标签 / 4 个成形点。
- **为什么算硬伤**：`message/result.go:9` 自己写着「字只在这一处成形（ADR-0032 第四条「唯一拼句处」）」；`result.go:28` 的 `NodeTag` 注释写「执行站也从这里取（它 import message，不自己拼）」。代码的事实与这两句相反，且后果可指认：改一次「错误详情」的说法，日志那一处改不到。
- **伤**：腐蚀架构（A7 的拼句边界被破坏，非「行为变了」——两处输出本来就分别属于不同产物，A1 逐字比对不覆盖日志内部一致性）。
- **档**：硬伤。

### F7【判断题】两套人读字节格式化：同一数量在同一次运行里两种写法

- **位置与证据**：
  - `internal/execute/transfer.go:201` `humanBytes` —— 按量级选单位、`trimFloat` 去尾零，输出 `51 MB`；用于空间预检报文（`transfer.go:145-155`，经 `judgeSpace:133`）。
  - `internal/message/confirm_rows.go:20` `Bytes` —— `"%.1f %cB"`，输出 `51.0 MB`；用于传输面板（`transfer_view.go:238`）与执行期日志（`log_node.go:73`、`:123`）。
  - 一次容量不足的上传：屏幕/日志写 `... 51.0 MB ...`，同一轮的 `ErrorText` 报文写 `... 51 MB available ...`。
- **本该用谁**：`message.Bytes`。老版同样是两份：`modules/SSHFleet_Go/internal/common/bytes.go:12` 的 `FormatBytes`（保留一位小数，供确认屏 `confirm/confirm.go:365`、面板 `output/progress.go:330`、日志 `output/reporter.go:255,301`）与 `ssh/space.go:149` 的 `humanBytes`（去尾零，供空间报文）。新工程把这两份原样搬成 `message.Bytes` + `execute.humanBytes`，**行为未变，重复也一起搬了**。
- **伤**：腐蚀架构（另造一套；代价是同一数量两种写法，读日志与读报文的人对不上）。
- **档**：判断题（改 `humanBytes` 会动 `ErrorText` 字节，属 A1 面，需评估）。

### F2／F3 见第三节（它们既是「另造一套」也是「站里拼话」，不重复计数，只在第三节算一次）

---

## 二、回读源头：有没有绕开交接件、直接回上游取数

**扫了 7 处候选**：R1 私钥按路径读（`client.go:135`）、R2 上传源文件重读（`run_upload.go:78,127,191,240`）、R3 生效并发重算（`run.go:61-64`）、R4 日志按 `RunParams` 反推解析站拼装（`log_session.go:111-179`）、R5 用 `c.node.User != "root"` 推执行身份（`run_upload.go:42`、`run_download.go:35`）、R6 判据表读 `p.Tables`（`verdict.go`／`run_passwd.go:288`／`run_answer.go:110`）、R7 上传/下载路径读 `p.Task`（`run_upload.go:34-41`、`run_download.go:33-34`）。

- **R2/R5/R6/R7 判为不是回读**：读的是纸上的字段（A12 允许「只装下一站要用的」），且 `c.node.User`／`p.Tables` 都是纸上写好的值，没有绕到包里或配置文件去取。
- **R1 登记不报**：纸带的是私钥**路径**、执行站按路径读文件（`client.go:135`，注释 `:6` 说明与老版差异：老版在清单那步就读成原文）。这是「纸带路径还是带原文」的取舍，读不到时挂 `connect_key_parse_failed`（`client.go:137`），已在阶段 05 的已知差异里；行为判据归 03，不重开。
- **R3 单独成条（F11）**：执行站把「生效并发」重算一遍并写回纸；这一格与确认屏的口径不等价。
- **报 2 处**：

### F11【硬伤】`-n` 大于台数时，确认屏印 100、报告印 5——同一轮里两个「并发数」

- **位置与证据**：
  - `internal/execute/run.go:61-64`：`limit := p.Run.Concurrency; if limit <= 0 || limit > len(p.Nodes) { limit = len(p.Nodes) }`，`:67` `paper.Concurrency = limit`。
  - `internal/message/confirm.go:165`：确认屏的并发数走 `concurrencyValue(p.Run.Concurrency, len(p.Nodes))`；`:175-180` 的 `concurrencyValue` 只有 `0 → 台数` 一条规则，**没有上限**。同一条规则在 `internal/gate/confirm.go:207-212` 的 `concurrencyInEffect` 里还有第三份（同样只有 `0 → 台数`）。`p.Run.Concurrency` 是解析站原样收下的 `-n`（`internal/parse/argv.go:163`），闸门不夹（`gate/confirm.go` 只改 `ConcurrencyAdvice`）。
  - 报告侧：`internal/settle/settle.go:42` 把 `paper.Concurrency` 抄进 `Ledger.Concurrency`，`message/stats.go:176` 再走同一个 `concurrencyValue`——此时值已经是 5。
  - 结论（读代码得出，未跑）：`-n 100` 配 5 台 → **确认屏「并发数： 100」、`report.txt`「并发数： 5」**，而实际起的是 5 个 worker。
- **与老版的差别**：老版报告印 `a.Number` 原值（`modules/SSHFleet_Go/internal/output/report.go:80`），而 `a.Number` 只在「没给 -n」时才被补成台数（`confirm/confirm.go:37-38`：`if args.Number == 0 { args.Number = nodes.Len() }`）——所以 `-n 100` 配 5 台时老版 `report.txt` 印 **100**，新版印 **5**。A1 明示覆盖 `report.txt`，这一格没有评估记录。
- **A/B 没覆盖这一格**：37 场的 `scenes.tsv` 只有一处带 `-n`（`c3-interrupt`，`-n 1`，台数 ≥1，没触发上限），**`-n` 大于台数的场景一场都没有**——所以这不是 03 的口径问题，是场景矩阵的空格（这一格归 03 记账，本轴只提供代码事实）。
- **伤**：行为变了（`report.txt` 的「并发数」在 `-n > 台数` 时与老版不同）＋ 同一次运行内两处显示不一致。
- **档**：硬伤。判它硬伤的理由是「同一次运行里确认屏与报告两个数」这条内部不一致——不管 A1 怎么算，这一点是事实错误；至于「报告印生效值」这个方向本身要不要保留，由开发者定（`contract/result.go:93` 的注释确实写着「实际生效」，所以方向有据，缺的是把确认屏也对齐、或把这一格的差异记录下来）。

### F6【判断题】日志开局把解析站的拼装过程按字段反推了一遍——拼装在一站、描述在另一站

- **位置与证据**：`internal/execute/log_session.go` 7 个函数全在做这件事：
  - `answersBlock:79-98`、`scriptBlock:102-108`、`downBlock:134-147` —— 交代「交给 SSH 执行的是什么」，逐行拼 `命令行 / stdin / 说明`；
  - `envNote:162-171` —— 从 `p.Run.Env`、`p.Run.English` 反推「先导入配置文件里的环境变量，再判断目标机编码、把提示语钉成英文，然后…」；
  - `scriptIdentity:111-116`、`identityLabel:118-127`、`commandWho:174-179` —— 从 `Sudo`／`NoShell`／`Interpreter` 反推执行身份与解释器。
  对照解析站的真正拼装点：`internal/parse/downlink.go:127-133`（`envHead` = `exportPrefix` + `localeFragment`）、`:117-123`（`sudoPrefix`）、`:55-90`（`downlinkOf` 四支）。老版这一整块与拼装**同住一个文件**：`modules/SSHFleet_Go/internal/ssh/command.go:159`（`BuildCommand`）与 `:253`（`DescribeCommand`）、`:291`（`envNote`）、`:307`（`commandWho`）。
- **后果**：解析站改 `envHead` 的组成（例如多一条 `export`、或换 locale 片段），日志仍按旧叙述写，而这段日志的自我定位是「事后唯一现场」（`log_session.go:7-10`）。当前两处措辞一致，所以这是「两处维护、尚未漂」，不是已发生的错。
- **伤**：腐蚀架构（A6/A12 的意图：交接件给成品，下一站不再回头解释上游怎么拼的）。行为未变。
- **档**：判断题。代价：搬回解析站会让日志描述与拼装同源，但 `log_session` 现在读的是整张纸（不只是 `Downlink`），搬动会牵到 05 的交接件形状——这是取舍点，不是笔误。

---

## 三、站里拼话：`execute` 里自己拼的中文句子

**计数（可复算）**：

```bash
cd modules/SSHFleet2_Go/internal/execute
LC_ALL=C.UTF-8 grep -c '"[^"]*[一-龥]' $(ls *.go | grep -v _test)   # 逐文件行数，相加 = 108
```

原始命中 **108 行**；其中 **38 行**是注释（行内或行尾的中文被引号夹住，逐条剔掉：`client.go:46`、`connect_message.go:50`、`message.go:22-28`、`stages.go:13-16,19-20`、`transfer.go:20-38,44-45`）。**净 70 行 / 54 处语句 / 8 个文件**。

| 文件 | 行 | 语句 | 成形方式 | 去向 |
| --- | --- | --- | --- | --- |
| `log_session.go` | 31 | 18 | `fmt.Sprintf` ＋ `+` 串接（最长一处 5 行拼一块） | 执行期日志 |
| `log_node.go` | 16 | 16 | `fmt.Sprintf` ／ `ip + "字" + 值` | 执行期日志 |
| `transfer_view.go` | 5 | 5 | `fmt.Sprintf` ／ `message.ActionName(mode) + "进度"` | 屏幕面板 |
| `run_upload.go` | 4 | 4 | `fmt.Sprintf("%s: 上传失败 - %s")` | `Output`（`terminal-output.txt`）＋ `ErrorText` |
| `run_download.go` | 4 | 4 | 同上，`"下载失败 - "` | 同上 |
| `progress.go` | 4 | 1 | 一条 `fmt.Sprintf` 里塞 5 个标签 | 屏幕面板 |
| `client.go` | 4 | 4 | 1 句 `fmt.Errorf` ＋ 3 个取值词（`密钥/密码/未配置`） | `ErrorText` ／ 日志「登录方式」 |
| `reporter.go` | 2 | 2 | `fmt.Errorf("建…失败（%s）：%w")` | `contract.CannotStart.Body` |
| **合计** | **70** | **54** | | |

按去向分：**执行期日志 34 句 ＋ 3 个取值词**；屏幕面板 6 句；`Output`/`ErrorText` 明细行 8 句；`CannotStart.Body` 2 句；`ErrorText` 里的老版原话 1 句。

**只登记、不判合规的两项**（§1.8）：

- `CannotStart.Body` 装的是站里拼好的中文整句：`reporter.go:42`「建终端输出文件失败（%s）：%w」、`:47`「建执行期日志失败（%s）：%w」，经 `run.go:49` 装进契约。**1 格 / 2 处**，落点「待开发者裁」。
- 报文格式的唯一定义在 `internal/execute/message.go`（66 行：`messagePrefix` ＋ 7 个环境常量 ＋ `msg()` ＋ `knownEnv()`），`msg(env…)` 调用点 **58 处 / 10 个文件**；`internal/message` 全包不认这个格式（全工程只有 `execute/message.go` 出现 `"sshfleet."`），`execute/message.go` 与 `internal/message` 同名不同物，`transfer_view.go` 里两种 `message` 同时出现（`message.ActionName` 与本地 `msg`）。这条归属是迁移地图定的（`message.go:5-7` 注释），**不是「没调 message 的现成能力」**；落点「待开发者裁」。

### F2【判断题】执行期日志的 34 句在 `execute` 内成形，`message` 只留了半截口子

- **位置**：`log_node.go:44,54,56,69,71,75,77,81,87,102,110,118,121,134,165,175`（16）＋ `log_session.go:37,40,80,82,87-90,92-95,103-106,113,115,126,138-140,143-146,163,165,168,170,176,178`（18）。
- **反证（为什么这不是「message 里没有，只能在这儿写」）**：`message/stats.go:356` 的 `ExecSummaryLog` 就是一条执行期日志句（`执行结束：成功 %d 台，失败 %d 台…`）——「日志句归 message」在本仓库已有先例；`message/result.go:45` 的 `ResultLineText` 注释写「执行期日志按自己的格式收尾」，而它的调用点在全工程是 **0**（只在 `ResultLine` 内部用）。也就是说 message 为日志留了口子，执行站没用，自己另写了一套五级版式。
- **已经半途而废的证据**：这 34 句里有一半是从 message 取的（`LogName`／`LogLabel`／`ActionName`／`Bytes`／`NodeTag`／`StepNote`／`UnknownIfEmpty`／`UploadSkippedText`／`AnswerDescribe`），另一半（`成功：`、`连接成功`、`连接失败`、`错误详情：`、`分类: `、`输出明细（%d 行）：`、`……其余 %d 行省略`、`执行参数：`、`开始执行任务：`、`交给 SSH 执行：`、`以 root 身份`、`先导入配置文件里的环境变量…`）自拼。同一文件里两种取法并存。
- **伤**：腐蚀架构（A7）。代价：改一句日志措辞要动 2 个文件、34 处；`message` 的 25 个生产文件按用途分（屏幕/行/标签/帮助/词表/宽度/系统提示），**没有「执行期日志的节点明细」这一类**——唯一一条日志句 `ExecSummaryLog` 寄在 `stats.go`（那份文件的主题是统计块与 `report.txt`），所以执行站要写节点明细时没处可调，就地写了一份。
- **档**：判断题。开发者 2026-11-02 说的「log 是记录的」只解释了**为什么日志与屏幕不合成同一段字**（`reporter.go:5-9`），没有授权「日志的字在站里成形」——这两件事在代码里被当成了一件。

### F3【判断题】屏幕面板的 6 句标签在 `execute` 内成形，同一文件一半取自 message

- **位置**：`progress.go:116-120`（一条 `Sprintf`：`总进度`、`已完成 %d/%d`、`成功 %d`、`失败 %d`）；`transfer_view.go:232`（`message.ActionName(mode) + "进度"`）、`:241`（`节点进度`）、`:243`、`:244`（`成功/失败 %d`）、`:255`（`共 %d 个文件  成功 %d  失败 %d`）。
- **证据（同一文件两种取法）**：`transfer_view.go` 已经 import message 并用 `ActionName`（`:232`）、`FormatSpeed`（`:238`、`:253`）、`Bytes`（`:238`）——即"模式名/速度/字节"取自 message，"进度/节点进度/成功/失败/共 N 个文件"自拼。`progress.go:9-10` 的注释自认「字本身仍只有两个来源：结果行由 message 造，进度帧的标签是面板的一部分」——这是一句**代码注释自己开的口子**，仓库里没有对应的 ADR。
- **伤**：腐蚀架构（A7）。代价同 F2；另有一处直接后果：`"进度"` 接在 `message.ActionName` 后面拼成「上传进度」，属跨包拼句，message 若要改「上传」二字的用法，这一处跟不到。
- **档**：判断题。

### F4【判断题】传输明细行 8 句在 `execute` 内成形，同一句文案两个文件各一份

- **位置**：`run_upload.go:98`（`"%s: 已跳过（符号链接）"`）、`:121`（`"上传失败 - "`）、`:130`、`:150`；`run_download.go:145`（与 `run_upload.go:98` **逐字相同**）、`:180`、`:188`、`:197`（`"下载失败 - "`）。
- **去向**：`res.Output`（进 `terminal-output.txt`）与 `res.ErrorText`（上屏、并参与分类匹配）。**这些行不是纯版面**：`ErrorText` 会进 `firstCategory`（`verdict.go:42,54`）做关键词匹配，措辞是判据文本的一部分——`run_upload.go:65` 的注释自己写过「两处各写一套就会有一条永远归不了类」。
- **伤**：腐蚀架构（A7）。代价：改一句明细措辞要动 2 个文件 8 处，且其中一处是判据文本。
- **档**：判断题。

---

## 四、按老版特判：只有老版才需要的分支

**扫了 19 处分支**：15 处按模式或载荷形态分的支（`step.go:25` 五支、`run.go:56`、`reporter.go:64,74`、`run_command.go:51`、`run_answer.go:21`、`verdict.go:63,86,130`、`log_node.go:64,119`、`log_session.go:49,62,134`、`progress.go:174`），4 处按自家文本格式分的支（`log_node.go:97` 的 `sameAsError`、`:168` 的输出行数上限、`:197` 的 `total_files=` 前缀、`run_command.go:198-200` 的 CR 归一）。只取模式名当标签、不分支的地方不计（`run.go:36`、`transfer_view.go:232`）。

**0 处判为「只有老版才需要」**——15 处按模式的支都对应现行五模式各自的语义（A13 明许按模式分文件/分支），没有一处是为兼容老命令名、老配置键或老字段名而留的。

**1 处按自家输出格式取的分支**：

### F10【判断题】日志按 `total_files=` 前缀静默丢行

- **位置与证据**：`log_node.go:192-203`，`:197` `if strings.TrimSpace(ln) == "" || strings.HasPrefix(ln, "total_files=") { continue }`。该前缀是传输明细的统计头（`transfer.go:227-233` 的 `buildTransferOutput` 生成 `total_files=%d, success_files=…`）。
- **老版同一处口径**：`modules/SSHFleet_Go/internal/output/reporter.go:495` 逐字相同（含注释）。所以**不是行为改变**。
- **代价**：命令/脚本模式下 `res.Output` 来自远端，远端输出里若有一行以 `total_files=` 开头，执行期日志会静默丢掉它（`terminal-output.txt` 与纸里的 `Output` 仍在）。这是"按自家文本格式取分支"的一个盲点：它不区分这一行是工具写的还是远端说的。
- **伤**：只是架构取舍（＋日志保真度的一处已知代价）。
- **档**：判断题。

---

## 五、长抽屉：22 个文件按行数排，前五个逐个说

**排序（生产 22 个，`wc -l`）**：`run_passwd 418`／`interactive 406`／`run_upload 308`／`run_download 262`／`transfer_view 258`／`transfer_progress 254`／`transfer 240`／`progress 231`／`run_command 219`／`log_node 211`／`run 207`／`verdict 202`／`client 185`／`log_session 179`／`run_answer 168`／`reporter 89`／`connect_message 77`／`message 66`／`step 42`／`remote 37`／`stages 21`／`doc 7`。

判据用 `docs/个人开发规范` 三.2「模块内各文件的用途应能被一句话说清」——**不是行数**：

| 文件 | 一句话 | 能说清？ | 里面堆了什么 |
| --- | --- | --- | --- |
| `run_passwd.go` 418 | 一台的批量改密 | 能 | 4 件事：步骤匹配器（`passwdMatcher` 5 个方法 / ~110 行）、写侧钩子、失败原因提取（`passwdReason`）、收尾判定与报文（`passwdStuckMessage`）；21 个函数 |
| `interactive.go` 406 | 代填与改密共用的交互底层 | 勉强 | 3 块**互不调用**：关键词匹配（`keywordRule`/`cursorMatcher`，~175 行）、采集（`outputTap`，~55 行）、会话驱动（`driveSession`/`startPTY`/`pump`/`stepsOf`，~125 行）。共同线索只有「边跑边喂」，没有 seam |
| `run_upload.go` 308 | 一台上传（直写／sudo 暂存／回滚） | 能 | `upload` 153 行（28-180）一条主循环 5 个 return 点；`putFile`／`putFileViaSudo`／`rollback` 各一段 |
| `run_download.go` 262 | 一台下载 | 能 | `download` 189 行（28-216）——**本包最长函数**，一条主循环 6 个出口 |
| `transfer_view.go` 258 | 传输面板的 bubbletea 模型 | 能 | 单一用途（`Model` 接口 4 方法 ＋ 测速 ＋ 渲染），不散 |

**长函数**（`个人开发规范` 对函数长度没有给硬指标，这里只给数）：`download` 189 行、`upload` 153 行、`Run` 125 行（`run.go:33-157`）、`passwd` 86 行、`logNode` 66 行。

### F9【判断题】两个 150+ 行的主循环函数＋`interactive.go` 三块混装

- **位置**：`run_download.go:28-216`（189 行）、`run_upload.go:28-180`（153 行）、`run.go:33-157`（125 行）、`interactive.go` 三块（见上表）。
- **为什么报**：5 个最大文件的用途都**能**一句话说清（所以不判「文件过长」），报的是其中两处：`download`/`upload` 各自把「预检 → 建目录 → 逐文件 → 失败分支 → 回滚/清半成品 → 汇总」串在一个函数体里，`download` 有 6 个 `return` 出口、`upload` 有 5 个，每个出口都要手抄一份「补 `Output`、补计数、补 `ErrorText`」的三行；`interactive.go` 的匹配/采集/驱动三块只共享「边跑边喂」这一个线索，拆成三份也不违反 A13（它允许按用途分文件）。
- **伤**：只是架构取舍（两个函数都能读懂、都有注释；代价是出口重复的那三行——`run_download.go:150-155` 与 `:176-181` 是同一组赋值的两次抄写）。
- **档**：判断题。

### F8【判断题】包内／跨包自重复 6 对（都不是「按模式分文件」，是同一件事写了两遍）

1. **同一段判断两处**：`run_passwd.go:287-294` `enterKeywords` 与 `run_answer.go:109-116` `abortKeywords`——循环体逐字相同（遍历 `p.Tables.Categories` 找 `message.CategoryPasswdExpired` 并返回 `Keywords`），只有函数名与注释不同。本该是一处。
2. **同结构两份**：`transfer_progress.go:171-193` `progressWriter` 与 `:195-222` `progressReader`——字段 8 个全同、节流逻辑相同，只有方法名 `Write`/`Read` 与累加字段名不同，各 23 行。
3. **同一组数算两遍**：`run.go:90-95,116-125` 的 `completed/succeeded/failed` 与 `transfer_progress.go:50-53,105-113` 的 `aggregator.completed/succeeded/failed`——同一份事实两套计数器（一套喂单行面板，一套喂传输面板）。
4. **同一句文案两份**：`run_upload.go:98` 与 `run_download.go:145` 的 `"%s: 已跳过（符号链接）"` 逐字相同（另见 F4）。
5. **同一转义两份**：`execute/remote.go:35-37` `escapeShellArg` 与 `parse/downlink.go:198` 的 `"'" + strings.ReplaceAll(s, "'", `'\''`) + "'"`——`'\''` 这个替换全工程写了 3 份（第 3 份是 `message/stats.go:254`，但那份按自己的注释是**有意不同**：能省引号就省，用于可照抄的命令行）。`remote.go:33` 的注释写「不再各写一遍」，与事实相反（`remote.go:36` 就是第二份）。
6. **同一段「取代填表」两处**：`execute/run_answer.go:21-29` 的 `answersOf`（`Task.Command.Answers` / `Task.Script.Answers`）与 `internal/gate/confirm.go:215-222` 的同名函数逐字同构——同一个名字、同一段逻辑，闸门与执行站各一份。

- **伤**：腐蚀架构（第 1、2、4、5、6 项）／只是架构取舍（第 3 项——两套计数器服务两种面板，合并要动并发路径）。
- **档**：判断题。代价分别为：第 1 项改口径要改两处且容易只改一处；第 2 项多 23 行等价代码；第 5、6 项是跨包重复（本轴只从 `execute` 这一侧报，`parse`／`gate` 侧不重复计，06／08 若报同一件事请合并）。

---

## 六、分母

| 项 | 核了多少 | 报了多少 |
| --- | --- | --- |
| 读生产文件 | **22 / 22**（4,087 行全读；`wc -l` 与基线 4,087 一致） | — |
| 另造一套 | **12 个能力面 / ~40 个调用点** | **2 条**（F1 硬伤、F7 判断）＋ F2／F3 计在第三节、不重复计；另 2 对包内自重复在 F8.1／F8.2 |
| 回读源头 | **7 处候选**（R1–R7） | **2 条**（F11 硬伤、F6 判断）；R1 登记不报，R2/R5/R6/R7 判为读纸上字段 |
| 站里拼话 | **70 行 / 54 处语句 / 8 个文件**（108 行原始命中 − 38 行注释） | **3 条**（F2 34 句、F3 6 句、F4 8 句）＋ 登记不判 2 项（`CannotStart.Body` 2 句、报文 58 个调用点） |
| 按老版特判 | **19 处分支**（15 按模式/载荷、4 按自家文本格式） | **1 条**（F10）；0 处判为「只有老版才需要」 |
| 长抽屉 | **22 个文件全排**，前 5 逐个说；长函数 5 个 | **1 条**（F9）；另 F8 的 6 对重复（4 对包内、2 对跨包） |
| 测试（旁证） | 19 个 `_test.go` 里**7 个**钉住了上面这些中文（`log_node`／`log_session`／`progress`／`reporter`／`run_command_integration`／`run_upload_integration`／`transfer_view`） | 不进分母 |

**合计报 10 条：硬伤 2（F1、F11）、判断题 8（F2、F3、F4、F6、F7、F8、F9、F10；编号跳过 F5，那一条是上文第三节「只登记、不判」的契约格装字，不计入条数）。** 那一条与报文格式归属（58 个调用点）落点均写「待开发者裁」，没有写成违规。

**锚质疑：无。** 没有发现 A7／A12／A1 与源码冲突到需要改锚的地方——F1／F2／F3／F6／F11 都是「实现没守住锚（或没守住 message 自己的注释）」，属代码发现，不是「这条锚要不要改」。

---

## 七、未覆盖面

- **全程静态读，没构建、没跑测试**：`go build`／`go test` 一次都没跑（§1.5「能静态读出来的不要为了跑一下去构建」）。所以「两处措辞一致」「包装形态对得上」全是读出来的，不是跑出来的；`download`/`upload` 的出口重复、`aggregator` 与 `run.go` 计数器是否真的一致，只按代码读判。
- **没上真机**：取消/超时/改密对话/传输中断的运行时行为一律没验（轮三）。
- **F11 是读代码得出的，没有真跑 `-n 100` 配 5 台**：跑它要写 `history/`，超出「唯一可写是回执」的授权。它成立靠四处代码路径（`run.go:61-67` → `settle.go:42` → `message/stats.go:176` → `message/confirm.go:165,175-180`）和两处老版对照（`output/report.go:80`、`confirm/confirm.go:37-38`），没有一处靠推测；但如果开发者要求「行为类必须有跑过的命令」，这一条要挪到轮三补跑。
- **19 个 `_test.go`（2,766 行）只扫了「有没有钉住中文」这一件事**（7 个文件命中），没逐个读断言；测试质量与红线归 02。
- **`internal/message` 只读了导出清单与 4 份文件**（`labels`／`stats`／`result`／`confirm_rows` 的相关函数），没逐文件读——字的成形全貌归 07；本轴引它只为判「本该用谁」。
- **`internal/parse` 只读了下发行那一段与建议并发那一个函数**（`downlink.go:1-133`，另 grep 了 `:198` 一行；`collect.go:124-143`），用来核对 F6、F8.5、F11；解析站包内质量归 06。
- **`internal/contract` 只读了用到的 4 个文件的字段注释**，没判契约本身（08）。
- **老工程只对照了 6 个文件的相关片段**（`ssh/command.go`、`ssh/space.go`、`ssh/connect_message.go`、`batch/batch.go`、`batch/pool.go`、`output/reporter.go`），没有通读 `ssh/interactive.go` 与 `ssh/passwdmatcher.go`——所以改密的匹配语义只核了新工程内部自洽，没做新老逐行对照。
- **`execute.MatchOptions`（`interactive.go:37`）是导出的，但包外 0 引用**：只记位置，不判——导出符号零下游归 08 重算（`符号级引用表.md` 的「84 个」不可信那条）。
- **没判** import 方向/五站单向（01）、红线与出口白名单（02）、契约格装字的合规性（未裁）、`message` 的字该不该这么分（07）、`parse` 侧同一件事（06）。
- **bubbletea 面板的视觉观感没验**：只读模型代码（`transfer_view`/`progress`），没渲染过一帧，宽度/降色/20 条上限的实际效果未看。

---

## 八、顺带（文档/注释面，不占三格）

- `internal/message/result.go:43-45`：`ResultLineText` 的注释写「执行期日志按自己的格式收尾」，暗示日志在用这个入口；全工程 0 调用点（只在 `ResultLine` 内部用）。注释描述的是一个没发生的分工。
- `internal/execute/remote.go:33`：「转义规则与解析站拼下发行那一处同源……不再各写一遍」——`remote.go:36` 就是第二份，且 `parse/downlink.go:198` 是第三处同类写法。注释与事实相反。
- `internal/execute/progress.go:9-10`：「字本身仍只有两个来源：结果行由 message 造，进度帧的标签是面板的一部分（老版也住在呈现层）」——「面板的标签属于面板」这个说法在仓库里找不到对应的 ADR，属代码注释自开的口子（F3 已按腐蚀报，此处只记它是一句无据的注释）。
- `internal/execute/message.go:3-7`：把一个文件和横切包同名（`message`），且注释里说「报文由执行侧产出，而执行侧不许反过来依赖判定包」——前半句是事实，后半句解释的是「为什么不放判定包」，没有解释「为什么不放横切 `message`」。
