# 报文分类议题 · 实施清单（工单 01 交付物）

Status: done

> **本文件是本议题唯一承载 `file:line` 的地方。** spec 与工单按 `docs/agents/doc-formats.md` 不写路径与片段，要定位到具体函数时看这里。
>
> 摸底基准：`cfb7ecc`（2026-10-20）工作区，`go build` / `go vet` 退出码 0，`go test ./...` 13 个包全 ok。
> 计数口径：**只数非测试代码**（`_test.go` 一律不计）；中文注释不计入「产出给使用者的文本」。
>
> 颗粒度按开工前定下的中粒度：02 要用的写到能直接动手的精度；测试断言只到「哪个文件、哪一类」，不到每条断言（03–06 各工单做到时再补）。

## 一、结论摘要

| 项 | 数 | 说明 |
| --- | --- | --- |
| 判定层中文分类常量 | **11** | 10 个改发报文；第 11 个「密码过期」不发文，见 §五 |
| 工具自编中文、**会落进报错原文** | **31 处** | `ssh` 27 + `batch` 1 + 其它 3，见 §二 |
| 工具自编中文、**只进输出明细** | **4 处** | 逐文件跳过行 / 上传失败行，**不归报文管** |
| 工具自编中文、**既不进原文也不进明细** | **3 处** | 建连前的前置错误，只经 `fatal` 上屏，见 §二·C |
| 判据文件里匹配自编中文的关键词 | **23 条** | 与已知量级一致，逐条见 §七 |
| 语料中 `error` 字段是自编中文的用例 | **17 条** | spec 口径正确；其中 `建立连接失败 - …` 是 **6 条**（spec 写的 7 条要更正） |
| 读「密码过期」这个分类名的地方 | **2 处** | 入场信号、代填中止词，见 §五 |
| SSH 库已分类却未用上的错误类型 | **3 类** | `ExitMissingError` / `PassphraseMissingError` / `OpenChannelError`，见 §六 |
| 写报错原文的语句 | **29 处赋值** | 其中 26 处直接 `strPtr(...)`、3 处经 `endFactsOf` 的 `f.errText`，见 §八 |
| 出厂报文 | **38 条** | 全部报文清单见 §九（M1–M38） |
| 测试文件涉及分类名 | **7 个** | 语义 6 个、措辞 2 个（有重叠），见 §十一 |

## 二、工具自编中文：逐处清单

判定「改还是不改」的判据是**会不会落进报错原文那一格**。落进去的才归报文管。

### A. 会落进报错原文（31 处，全部要改）

**`internal/ssh`（27 处）**

| # | 位置 | 现行文本 | 判定性质 | 报文 |
| --- | --- | --- | --- | --- |
| A1 | `client.go:76` | `建立连接失败 - %w`（拨号失败） | 文本判定 | `connect.refused` / `connect.timeout` / `connect.no_route`，按 `err` 分支（`dns_failed` 已于审计后删除：清单只收 IP） |
| A2 | `client.go:85` | `建立连接失败 - 握手超时%v` | 文本判定 | `connect.handshake_timeout` |
| A3 | `client.go:87` | `建立连接失败 - %w`（握手失败） | 文本判定 | `connect.auth_failed`（`unable to authenticate` 等）或 `connect.handshake_failed` |
| A4 | `client.go:133` | `解析密钥失败 - %w` | 文本判定 | `connect.key_parse_failed` |
| A5 | `exec.go:86` | `创建会话失败 - ` + err | 文本判定 | `session.create_failed` |
| A6 | `exec.go:98` | `创建输入通道失败 - ` + werr | 文本判定 | `session.channel_failed` |
| A7 | `exec.go:182` | `命令执行超时(%v)` | 文本判定 | `run.timeout` |
| A8 | `interactive.go:163` | `创建会话失败 - ` + err | 文本判定 | `session.create_failed` |
| A9 | `interactive.go:171` | `申请终端失败 - ` + err | 文本判定 | `session.pty_failed` |
| A10 | `interactive.go:177` | `创建输入通道失败 - ` + err | 文本判定 | `session.channel_failed` |
| A11 | `interactive.go:292` | `命令执行超时(%v)` | 文本判定 | `run.timeout`（与 A7 **必须收成同一条**） |
| A12 | `passwdsession.go:57` | `创建会话失败 - ` + err | 文本判定 | `session.create_failed` |
| A13 | `passwdsession.go:65` | `申请终端失败 - ` + err | 文本判定 | `session.pty_failed` |
| A14 | `passwdsession.go:70` | `创建输入通道失败 - ` + err | 文本判定 | `session.channel_failed` |
| A15 | `sftp.go:44` | `SFTP 客户端创建失败 - ` + err（上传） | 文本判定 | `transfer.client_failed` |
| A16 | `sftp.go:192` | `SFTP 客户端创建失败 - ` + err（下载） | 文本判定 | `transfer.client_failed`（与 A15 同一条） |
| A17 | `sftp.go:63` | `远程路径不存在: %s`（上传） | 文本判定 | `transfer.remote_missing` |
| A18 | `sftp.go:67` | `远程目标路径不是目录: %s` | 文本判定 | `transfer.remote_not_dir` |
| A19 | `sftp.go:75` | `本地文件不存在或不可读: %s - %v` | 文本判定 | `transfer.local_unreadable` |
| A20 | `sftp.go:208` | `远程路径不存在: %s`（下载） | 文本判定 | `transfer.remote_missing`（与 A17 同一条） |
| A21 | `sftp.go:214` | `远程路径不可访问: %s` | 文本判定 | `transfer.remote_unreachable` |
| A22 | `sftp.go:251` | `获取远程文件列表失败: %v` | 文本判定 | `transfer.list_failed` |
| A23 | `sftp.go:283` | `没有可下载的文件：目标全部为软链接（已过滤 %d 个）` | 文本判定 | `transfer.no_files` |
| A24 | `sftp.go:285` | `远程路径中没有可下载的文件` | 文本判定 | `transfer.no_files`（与 A23 同一条，附原因） |
| A25 | `sftp.go:292` | `创建本地目录失败: %v` | 文本判定 | `transfer.local_dir_failed`（**不在 spec 清单里，要补**） |
| A26 | `sftp.go:114` | `%s: 上传失败 - 文件已存在` | 文本判定 | 逐文件行，见 §二·B 的说明：它同时进明细与原文 |
| A27 | `batch/collect.go:73` | `文件不可读: %s: %w` | **不是文本判定**（进致命错误路径，见 A31） | **不发报文**；与 A31 同一档 |

**`internal/ssh` 里 15 处 `fmt.Errorf` 拼出来的逐文件失败原因**（`sftp.go:380/386/392/406/420/426/433/442/456/466/484/490/504` 共 13 处，加 `sftp.go:126/151/331/346` 四处包装）：它们经 `msg := fmt.Sprintf("%s: 上传/下载失败 - %v", rel, err)` 写进 `result.Error`（A26 同形），是该文件的**最后一条失败原因**。改造后这些 `fmt.Errorf` 的中文全部换成报文（见 §九 `transfer.*`），逐文件行的 `%s: 上传失败 - <报文>` 结构不变。

**`internal/batch`（1 处）**

| # | 位置 | 现行文本 | 判定性质 | 报文 |
| --- | --- | --- | --- | --- |
| A28 | `collect.go:73` | `文件不可读: %s: %w` | **不是文本判定**（同 A27 / A31） | **不发报文** |

**其它包（3 处，经 `endFactsOf` 或参数校验进入报错原文）**

| # | 位置 | 现行文本 | 判定性质 | 报文 |
| --- | --- | --- | --- | --- |
| A29 | `ssh/client.go:144` | `未提供有效的认证方式（密码或密钥至少提供一种）` | 文本判定（但**属参数/配置错误**） | **不改**，见 §四 |
| A30 | `ssh/matcher.go:25` | `触发词不是合法正则：%s` | 文本判定（**启动期**） | **不改**，见 §四 |
| A31 | `batch/collect.go:*` | `file_path 必须是绝对路径` / `file_path 不存在或无法访问` / `没有可上传的文件…` / `不支持的文件类型…` | **不是文本判定**（在确认屏之前 `fatal` 退出，**不产生 `Result`**） | **不改**，见 §四 |

> **A31 的判定依据**：`main.go:364-366` 采集上传清单失败即 `fatal("upload", ...)`，此时一个节点都还没跑，没有 `Result.Error` 可写。这类错误进不了统计与分类，属「不归报文管」。

### B. 只进输出明细（4 处，不归报文管）

| # | 位置 | 文本 | 去处 |
| --- | --- | --- | --- |
| B1 | `sftp.go:89` | `%s: 已跳过（符号链接）`（上传） | `Output`（逐文件行） |
| B2 | `sftp.go:301` | `%s: 已跳过（符号链接）`（下载） | `Output`（逐文件行） |
| B3 | `reporter.go:378` | `未送出的代填：…` | 呈现层合成，不是执行侧产出 |
| B4 | `reporter.go:415-418` | `%s 未拿到退出码` / `%s 退出码 %d`（中间步骤） | 呈现层合成 |

**B3/B4 的归属**：它们是**呈现层陈述事实**，不进 `Result.Error`，也不参与判据匹配——保持中文，见 §四。

### C. 关键区分：同一句、两个去处

`sftp.go:126`（`上传失败`）、`sftp.go:331/346`（`下载失败`）三处是**双写**：既 append 进 `Output` 的逐文件行，又写进 `result.Error`。改造后：
- 进 `Output` 的那份：保持现有中文结构，中间的失败原因换成报文；
- 进 `result.Error` 的那份：就是 A 类的「最后一条失败原因」，报文由执行侧生成。

## 三、判定层：11 个中文分类常量逐个拆解

位置：`internal/verdict/verdict.go:38-69`。读字段的判定全部发生在 `Judge`（`verdict.go:79-202`）。

| 常量 | 分类名 | 读哪些字段 | 判定性质 | 改造后报文 |
| --- | --- | --- | --- | --- |
| `CategoryPartialSuccess` | 部分成功 | `FailedFiles > 0 && SuccessFiles > 0`（`verdict.go:112`） | **结构判定** | `transfer.finished`（执行侧收尾发） |
| `CategoryTriggerMiss` | 触发词未命中 | `SessionBegun && !Canceled && len(AnswersMissed) > 0`（`verdict.go:94`） | **结构判定** | `answer.missed`（代填收尾发） |
| `CategoryExhaustedTimeout` | 代填用尽后超时 | `SessionBegun && AnswersExhausted && TimedOut`（`verdict.go:99`） | **结构判定** | `answer.exhausted_timeout` |
| `CategoryNotExpired` | 密码未过期 | `cleanStop && !EnterSignalSeen`（`verdict.go:130`） | **结构判定** | `passwd.not_expired`（改密收尾发） |
| `CategoryNoPrompt` | 提示词没认出 | `Error == "" && !PasswdFedAny && passwdExitedNonzero`（`verdict.go:158`） | **结构判定** | `passwd.no_prompt` |
| `CategoryNoSignal` | 没等到提示 | `TimedOut && !PasswdFedAny`（`verdict.go:153`） | **结构判定** | `passwd.no_signal` |
| `CategoryNoReply` | 远端无响应 | `TimedOut && PasswdFedAny`（`verdict.go:152`） | **结构判定** | `passwd.no_reply` |
| `CategoryGiveUp` | 改密未完成 | `PasswdEarlyClose`（`verdict.go:148`）**或** `Error == "" && passwdExitedNonzero`（`verdict.go:162`） | **结构判定**（两条分支） | `passwd.give_up`（两处合一条） |
| `CategoryCanceled` | 任务已取消 | `Canceled && ConnectSuccess`（`verdict.go:175-177`） | **结构判定** | `abort.canceled`（建连收尾 / 各通道收尾发） |
| `CategoryConnectCanceled` | 建连被取消 | `Canceled && !ConnectSuccess`（`verdict.go:178`） | **结构判定** | `abort.connect_canceled`（建连收尾发） |
| `CategoryPasswordExpired` | **密码过期** | `AbortLine != ""`（`verdict.go:89`） | 文本判定（但名字由代码给） | **不发报文**，见 §五 |

**`CleanStop` 那条不在此列**：`verdict.go:127-128` 的 `derefString(res.Error) == ""` 读的是「报错原文空不空」，但**改造后保留**——报文只在非成功形态下写，它读作「没有事实要陈述」（spec《判定》已定）。它不是给分类名的路径。

**`passwdExitedNonzero`**（`verdict.go:238-245`）读 `Steps` 里 `StepPasswd` 的退出码，是结构判定，改造后**保留**——但它不再是「挑分类名」的依据，而是被收编进纯函数的入参（见 §十）。

## 四、不归报文管的点（保持中文）

| 位置 | 文本 | 为什么保持 |
| --- | --- | --- |
| `cli/args.go`、`cli/check.go`、`cli/answer.go`、`cli/passwd.go` 全量 | 参数校验错误（`不认识的选项：%s`、`-p 参数必须是以 / 结尾的目录…` 等） | 参数校验，不参与分类，也不是给人看的失败原因 |
| `ssh/matcher.go:25` | `触发词不是合法正则：%s` | 启动期校验，`fatal` 上屏 |
| `ssh/client.go:144` | `未提供有效的认证方式…` | 配置/参数错误，建连前就返回，进的是 `Connect` 的错误值，但它是**调用方拼错了配置**，不是目标机的事 |
| `cli/check.go` 的 `-u` / `-p` 本地检查 | `-p 参数指定的本地路径必须是已存在的目录…`、`-u 参数指定的上传文件或目录不存在…` | 参数阶段的本地检查，`fatal` 上屏，不产生 `Result`；开工前的本地检查不归类 |
| `batch/collect.go` 全部 | `file_path 必须是绝对路径`、`没有可上传的文件…`、`不支持的文件类型…` | 采集期错误，`fatal` 退出，不产生 `Result`；开工前的本地检查不归类 |
| `main.go` 全量 | 启动期日志、`fatal` 提示 | 工具日志 / 致命错误，不参与分类 |
| `output/reporter.go:378`、`415-418` | `未送出的代填：…`、`中间步骤…退出码…` | 呈现层陈述事实，不进判据匹配 |
| `sftp.go:89`、`301` | `已跳过（符号链接）` | 逐文件跳过行，进输出明细 |
| `verdict/keywords.go`、`config/passwd.go` 全部 | 配置加载/校验错误 | 启动期，`fatal` 上屏 |

## 五、「密码过期」：谁在用它、真正需要的是什么

| 用途 | 位置 | 现在怎么取 | 真正需要的东西 |
| --- | --- | --- | --- |
| 改密**入场信号** | `batch/batch.go:181` → `ssh.PasswdInput.EnterKeywords` | `kw.PasswordExpired()` | **远端提示的关键词**，不是分类名 |
| 代填**中止词** | `batch/batch.go:181` 同一句，注入代填分支 | 同上 | 同上 |
| 启动期**守卫** | `main.go:431` | `len(errorKeywords.PasswordExpired()) == 0` | 同一批关键词非空 |
| 判定层**常量** | `verdict.go:41` | `CategoryPasswordExpired = "密码过期"` | **不需要常量**——它是常量唯一的消费者 |
| 按名查表的**入口** | `verdict/keywords.go:146-148` | `k.KeywordsOf(CategoryPasswordExpired)` | 字面量 `"密码过期"`（保留，不能跟着常量一起删） |

**结论**：它们要的是「远端那句提示的关键词」，不是分类名由代码给出。删常量时，把 `keywords.go:147` 的 `CategoryPasswordExpired` 换成一个**不导出的字面量**，并在注释里写明「改分类 `name` 会让这里落空、须同步改字面量」。

**守卫行为要写进用例**（工单 05 的验收项）：改 `name` → 入场信号取空 → `main.go:431` 的守卫报错退出，而不是静默等到超时。这是**既有行为**，测试现在**没有覆盖**。

## 六、SSH 库已分类却未用上的错误

| 错误类型 | 现在 | 按 ADR-0021 第 8 条应翻成 | 调用点 |
| --- | --- | --- | --- |
| `*ssh.ExitMissingError` | **被丢弃**；靠判据表关键词 `remote command exited without exit status` 认库的提示原文（`keywords_error.conf:246`） | `sshfleet.session.exit_missing: remote command exited without exit status` | `exec.go:188`（`extractExitCode` 只认 `*ssh.ExitError`） |
| `*ssh.PassphraseMissingError` | 已用上（`client.go:127-131`），但文本是自编中文 | `sshfleet.connect.key_passphrase_missing: …` | `client.go:127` |
| `*ssh.OpenChannelError` | 未单独识别，退化为文本匹配 `ssh: pty-req failed` / `ssh: rejected` | 按 `Reason` 翻成 `session.pty_failed` / `session.create_failed`，原始 `Message` 跟在后面 | `exec.go:84`、`interactive.go:170`、`passwdsession.go:64` |

**`extractExitCode` 的改法**：`exec.go:188-195` 只认 `*ssh.ExitError`，改造后要**先认 `*ssh.ExitMissingError`**（翻成报文再走原文），否则库的提示改了措辞这条分类就静默失效。

## 七、判据文件里匹配自编中文的 23 条关键词

来源：`config/keywords_error.conf`（`keywords_passwd.conf` 里没有）。

| 分类 | 关键词 | 位置 | 对应程序字面量 |
| --- | --- | --- | --- |
| 握手超时 | `握手超时` | `keywords_error.conf:71` | `client.go:85` |
| 密钥解析失败 | `解析密钥失败` | `:88` | `client.go:133` |
| 密钥解析失败 | `这把私钥有口令` | `:89` | `client.go:130` |
| 执行超时 | `命令执行超时` | `:159` | `exec.go:182`、`interactive.go:292` |
| 远程路径不存在 | `远程路径不存在` | `:284` | `sftp.go:63`、`sftp.go:208` |
| 远端没有文件 | `远程路径中没有可下载的文件` | `:292` | `sftp.go:285` |
| 远端没有文件 | `没有可下载的文件` | `:293` | `sftp.go:283` |
| 远程文件已存在 | `文件已存在` | `:301` | `sftp.go:114` |
| SFTP失败 | `创建远程文件失败` | `:310` | `sftp.go:392` |
| SFTP失败 | `写入远程文件失败` | `:311` | `sftp.go:406` |
| SFTP失败 | `写入临时文件失败` | `:312` | `sftp.go:456` |
| SFTP失败 | `创建临时目录失败` | `:313` | `sftp.go:426` |
| SFTP失败 | `创建临时文件失败` | `:314` | `sftp.go:442` |
| SFTP失败 | `创建远程目录失败` | `:315` | `sftp.go:380`、`sftp.go:420` |
| SFTP失败 | `打开远程文件失败` | `:316` | `sftp.go:484` |
| SFTP失败 | `SFTP 客户端创建失败` | `:317` | `sftp.go:44`、`sftp.go:192` |
| sudo失败 | `sudo mv 失败` | `:325` | `sftp.go:466` |
| 本地文件错误 | `打开本地文件失败` | `:333` | `sftp.go:386`、`sftp.go:433` |
| 本地文件错误 | `创建本地文件失败` | `:334` | `sftp.go:490` |
| 本地文件错误 | `写入本地文件失败` | `:335` | `sftp.go:504` |
| 本地文件错误 | `文件不可读` | `:336` | `batch/collect.go:73`（**不是 `Result.Error`**，已撤，见下） |
| 无法建立会话 | `创建会话失败` | `:236` | `exec.go:86`、`interactive.go:163`、`passwdsession.go:57` |
| 无法建立会话 | `创建输入通道失败` | `:237` | `exec.go:98`、`interactive.go:177`、`passwdsession.go:70` |
| 无法建立会话 | `申请终端失败` | `:238` | `interactive.go:171`、`passwdsession.go:65` |

**计数说明**：表里 24 行，其中 `这把私钥有口令` 属**口令缺失**（`PassphraseMissingError`），与 `解析密钥失败` 是两条分支——按 spec 的 23 条口径，「密钥解析失败」只算 1 条，故合计 **23 条**（spec 分法：SFTP失败 8 + 本地文件错误 4 + 无法建立会话 3 + 远端没有文件 2 + 其余 6 各 1）。

**核实结论**：这 23 条与程序里的字面量逐条对上了，**没有遗漏**。但发现一处 spec 没提到的现象：`本地文件错误` 的 `文件不可读` 那条**对应的不是 `Result.Error`**（`collect.go` 在采集期 fatal），改造后它会**永远匹配不上**——因为它指向的文本根本进不了报错原文。

> **2026-10-20 处置（04 定）**：**撤掉**，不是扩 spec。口径三条：
> 1. **开工之前的本地检查不归类**：`cli/check.go` 的参数阶段（`-u` 存在性 / 符号链接、下载 `-p` 必须是已存在目录）与 `batch/collect.go` 的采集期，失败即 `fatal` 退出，一个节点都还没跑，没有 `Result.Error` 可写，不进统计与分类。`文件不可读` 这条关键词匹配的正是这一档的文本，本就不可能命中。
> 2. **开工之后的本地失败保留兜底**：本地这块预判不全（`os.Stat` 只问在不在，开工后仍会被删、被锁、被移走），上传逐文件 `open`、下载建目录 / 建文件失败照旧发 `transfer.local_unreadable` / `local_dir_failed` / `local_file_failed`，`本地文件错误` 分类继续认领。**不以「参数阶段查过了」为由取消**——那会把真实的运行时失败落到未归类。
> 3. **两档的界线是「节点开没开工」**，不是「本地还是远端」。前者没有 Result 可写，后者有。


## 八、`result.Error` 赋值点（实测 29 处）

### 逐点清单

`exec.go:54`（`connectFor`）、`exec.go:86`、`exec.go:98`、`exec.go:118`（经 `f.errText`）、`interactive.go:163/171/177/206`（`206` 经 `f.errText`）、`passwdsession.go:57/65/70/128/133`（`133` 经 `f.errText`）、`sftp.go:44/63/67/75/116/128/153/192/208/214/251/283/285/292/333/348`。

其中 **3 处是 `= f.errText`**（`exec.go:118`、`interactive.go:206`、`passwdsession.go:133`），走 `endFactsOf` 那一路；其余 26 处是直接 `strPtr(...)`。

### 经 `endFactsOf` 的 3 处（三条通道的唯一收尾映射点）

| 通道 | 调用点 | 结果去向 |
| --- | --- | --- |
| 命令 | `exec.go:116-120` | `result.Error = f.errText` |
| 代填 | `interactive.go:204-208` | 同上 |
| 改密 | `passwdsession.go:101-103` + `:124-135` | 同上 |

`endFactsOf`（`exec.go:216-232`）是 `命令执行超时(Ns)` 的第二份来源（A7/A11 的 `fmt.Errorf` 才是第一份）——**报文只在这里加一次前缀**，两个调用点不再各自拼（工单 03 Comments 已定）。

### 改造后「整句换」还是「追加」

| 语句组 | 处理 |
| --- | --- |
| `exec.go:86/98`、`interactive.go:163/171/177`、`passwdsession.go:57/65/70` | **整句换成报文**（此时原文只有这一句） |
| `sftp.go:44/63/67/75/114/126/151/192/208/214/251/283/285/292/331/346` | **整句换成报文**，但**逐文件失败行**（`126/151/331/346`）与 `Output` 那份共用同一份文本 |
| `connectFor`（`exec.go:52-64`） | **整句换成报文**（`client.go` 里那三处 `建立连接失败` 改成报文后，这里只是一次复制） |
| **结构性分类的报文**（`transfer.finished`、`passwd.*`、`answer.*`、`abort.*`） | **必须追加，不许覆盖**——这些时刻 `Error` 里可能已经有权威原文（传输的最后一条逐文件失败、建连被中断时的 `dial tcp …`）。顺序：原文在前、报文在后，换行分隔 |
| `endFactsOf` 的 `f.errText` | 把 `err.Error()` 包成报文（`run.timeout` / 其它），**唯一一次加前缀** |

**一处都不能覆盖**：§九里标 `追加` 的报文，改造时必须有测试证明「原有原文还在」。

## 九、出厂报文全集（38 条）

spec《预算：全部报文清单》的核准与补全版。**本表是「出厂报文有哪些」的唯一来源。**

| # | 报文 | Go | 发报点（包 / 函数） | 触发条件 | 写法 |
| --- | --- | --- | --- | --- | --- |
| M1 | `sshfleet.connect.refused` | `connect` | `ssh.Client.Connect`（`client.go:76`） | `err` 含 `connection refused/reset` | 整句 |
| M2 | `sshfleet.connect.timeout` | `connect` | 同上 | `net.Error.Timeout()` 且非握手阶段 | 整句 |
| M3 | `sshfleet.connect.no_route` | `connect` | 同上 | `no route to host` 族 | 整句 |
| M4 | ~~`sshfleet.connect.dns_failed`~~ | — | — | **已删除**（清单只收 IP，2026-10-20 审计后） | — |
| M5 | `sshfleet.connect.handshake_timeout` | `connect` | `Connect`（`client.go:84-86`） | 握手超时 | 整句 |
| M6 | `sshfleet.connect.auth_failed` | `connect` | `Connect`（`client.go:87`） | `unable to authenticate` 族 | 整句 |
| M7 | `sshfleet.connect.handshake_failed` | `connect` | `Connect`（`client.go:87` 兜底） | 其余握手失败（含 EOF） | 整句 |
| M8 | `sshfleet.connect.key_parse_failed` | `connect` | `buildAuthMethods`（`client.go:133`） | 私钥解析失败 | 整句 |
| M9 | `sshfleet.connect.key_passphrase_missing` | `connect` | `buildAuthMethods`（`client.go:127-131`） | `*ssh.PassphraseMissingError` | 整句 |
| M10 | `sshfleet.session.create_failed` | `session` | `exec.go:86`、`interactive.go:163`、`passwdsession.go:57` | `NewSession` 失败 | 整句 |
| M11 | `sshfleet.session.pty_failed` | `session` | `interactive.go:171`、`passwdsession.go:65` | `RequestPty` 失败 | 整句 |
| M12 | `sshfleet.session.channel_failed` | `session` | `exec.go:98`、`interactive.go:177`、`passwdsession.go:70` | `StdinPipe` 失败 | 整句 |
| M13 | `sshfleet.session.exit_missing` | `session` | `extractExitCode`（`exec.go:188`） | `*ssh.ExitMissingError` | 整句 |
| M14 | `sshfleet.run.timeout` | `run` | `endFactsOf`（`exec.go:216`），由 `runWithTimeoutAndCancel`（`exec.go:182`）与 `driveSession`（`interactive.go:292`）触发 | 执行超时 | 整句 |
| M15 | `sshfleet.run.rejected` | `run` | `endFactsOf` | `Start` 被拒（`ssh: command … failed`） | 整句 |
| M16 | `sshfleet.transfer.client_failed` | `transfer` | `sftp.go:44`、`sftp.go:192` | `sftp.NewClient` 失败 | 整句 |
| M17 | `sshfleet.transfer.remote_missing` | `transfer` | `sftp.go:63`、`sftp.go:208` | 远程路径不存在 | 整句 |
| M18 | `sshfleet.transfer.remote_not_dir` | `transfer` | `sftp.go:67` | 目标不是目录 | 整句 |
| M19 | `sshfleet.transfer.remote_unreachable` | `transfer` | `sftp.go:214` | `Stat` 失败 | 整句 |
| M20 | `sshfleet.transfer.list_failed` | `transfer` | `sftp.go:251` | 枚举 `find` 失败 | 整句 |
| M21 | `sshfleet.transfer.no_files` | `transfer` | `sftp.go:283`、`sftp.go:285` | 无可传文件 | 整句 |
| M22 | `sshfleet.transfer.local_unreadable` | `transfer` | `sftp.go:75`（预检）、逐文件 `open` 失败（`sftp.go:386/433`） | 本地文件不可读 | 整句 |
| M23 | `sshfleet.transfer.local_dir_failed` | `transfer` | `sftp.go:292`（建本地 IP 目录） | **spec 未列，本表补上** | 整句 |
| M24 | `sshfleet.transfer.remote_file_failed` | `transfer` | `sftp.go:380/392/406/420`（上传路径） | 建目录 / 建文件 / 写文件失败 | 整句 |
| M25 | `sshfleet.transfer.local_file_failed` | `transfer` | `sftp.go:490/504`（建本地文件 / 写本地文件失败） | 下载路径的本地侧失败 | 整句 |
| M26 | `sshfleet.transfer.tmp_failed` | `transfer` | `sftp.go` sudo 路径 | 临时目录 / 文件 / 写临时文件失败 | 整句 |
| M27 | `sshfleet.transfer.sudo_failed` | `transfer` | 同上 | `sudo mv` 到最终位置失败 | 整句 |
| M28 | `sshfleet.transfer.remote_exists` | `transfer` | 上传逐文件检查 | 目标机已有同名文件（不覆盖） | 逐文件行 |
| M29 | `sshfleet.transfer.finished` | `transfer` | 上传 / 下载 / 取消早退共用的收尾（见 §十） | 有成功也有失败 | **追加** |

> **M29 已退役（2026-10-20）**：上传改为「失败即回滚」（ADR-0022）后，传输只剩全成 / 全不成两种形态，`finished` 不再产出；接替它的是 `sshfleet.transfer.rolled_back`（已清干净）与 `sshfleet.transfer.rollback_partial`（没清干净，报文带剩余路径）。表内保留原行是留档当时摸底的事实，最终清单以代码 `stages.go` 与 `keywords_error.conf` 为准。
| M30 | `sshfleet.passwd.not_expired` | `passwd` | `RunPasswdChange` 收尾 | 干净收场但无入场信号 | **追加** |
| M31 | `sshfleet.passwd.no_signal` | `passwd` | 同上 | 超时且没喂过 | **追加** |
| M32 | `sshfleet.passwd.no_reply` | `passwd` | 同上 | 超时且喂过 | **追加** |
| M33 | `sshfleet.passwd.no_prompt` | `passwd` | 同上 | 非 0 收场、未喂过、远端没说话 | **追加** |
| M34 | `sshfleet.passwd.give_up` | `passwd` | 同上 | 早收场，或非 0 收场且喂过但无原话 | **追加** |
| M35 | `sshfleet.answer.missed` | `answer` | `RunInteractive` 收尾（`interactive.go:210-221`） | 有未送出的代填 | **追加** |
| M36 | `sshfleet.answer.exhausted_timeout` | `answer` | 同上 | 表已空仍超时 | **追加** |
| M37 | `sshfleet.abort.canceled` | `abort` | 建连收尾（`connectFor`）与三条通道收尾 | 已连上被中断 | **追加** |
| M38 | `sshfleet.abort.connect_canceled` | `abort` | `connectFor` | 未连上被中断 | 整句 |

> **合计 38 条（过程留档；终值 41 条）**：M1–M38 之外还漏了 `connect.dial_failed`（拨号失败兜底，见 `connect_message.go`），加上 M28 之后的 `rolled_back` / `rollback_partial`，代码现行环节常量为 **41 条**（connect 9 / session 4 / run 2 / transfer 17 / passwd 5 / answer 2 / abort 2；09 工单新增 `partial_left` / `local_partial_left`，`dns_failed` 已删）。**终值以代码常量与判据表为准**，本表只记下当时摸底的事实。与 spec《预算：全部报文清单》的框架表相比，**本表补上了 13 条**，都是摸代码与实施时实际找到的失败形态：`connect.handshake_failed`、`connect.key_passphrase_missing`、`session.exit_missing`、`run.rejected`、`transfer.remote_file_failed`、`transfer.local_file_failed`、`transfer.tmp_failed`、`transfer.sudo_failed`、`transfer.remote_exists`、`transfer.local_dir_failed`、`transfer.remote_not_dir`、`transfer.remote_unreachable`、`transfer.list_failed`。spec 说「01 逐条核实并补全，本表给的是框架」，指的就是这里。
>
> **`transfer` 那 13 处 `fmt.Errorf` 的归并已于 02 定案**：按「上传路径 / 下载路径 / sudo 路径」归成 3 条（`remote_file_failed` / `local_file_failed` / `tmp_failed`），原始错误跟在冒号后保留区分度。`sudo mv 失败` 另立 `sudo_failed`（判据表「sudo失败」就认它）。这个决定 04 不需要再定，核实即可。

## 十、结构形态 → 纯函数（导出的测试接缝）

每摊一个**导出**纯函数，入参是布尔与计数（不是整个 `Result`），产出一行报文文本（或空串表示「无事可陈」）。

| 环境 | 建议函数名 | 入参 | 发报的调用点 | 出参示例 |
| --- | --- | --- | --- | --- |
| `transfer` | `FinishedMessage(success, failed, total int) string` | `SuccessFiles` / `FailedFiles` / `TotalFiles` | `sftp.go:169`（上传收尾）、`sftp.go:363`（下载收尾）、`sftp.go:100/308`（取消早退） | `sshfleet.transfer.finished: 6 of 10 files transferred` |
| `passwd` | `PasswdMessage(fed, earlyClose, timedOut, sawEnter, hasRemoteReason bool) string` | `PasswdFedAny` / `PasswdEarlyClose` / `TimedOut` / `EnterSignalSeen` / `Error != ""` | `passwdsession.go:124` 之前 | `sshfleet.passwd.no_reply: password fed but no reply before timeout` |
| `answer` | `AnswerMessage(missed int, exhausted, timedOut bool) string` | `len(AnswersMissed)` / `AnswersExhausted` / `TimedOut` | `interactive.go:210-221` | `sshfleet.answer.missed: 2 answers not delivered` |
| `abort` | `AbortMessage(canceled, connected bool) string` | `Canceled` / `ConnectSuccess` | `connectFor`（`exec.go:60`）、三条通道收尾 | `sshfleet.abort.canceled: interrupted after session established` |

**成功规则要加的那一条（只有命令 / 脚本 / 代填）**：`Judge` 里 `verdict.go:103` 的成功判断之后补一句——`!strings.Contains(derefString(res.Error), "sshfleet.")`。传输与改密两条成功规则不动（前者要求 `FailedFiles == 0`，后者 `cleanStop` 要求原文为空）。

**「报错原文是不是空的」改造后的去向**：

| 位置 | 现状 | 改造后 |
| --- | --- | --- |
| `verdict.go:127-128` `cleanStop` | 读 `Error == ""` | **保留**（读作「没有事实要陈述」） |
| `verdict.go:158` 提示词没认出 | 读 `Error == ""` | **消失**，由 `passwd.no_prompt` 的触发条件接管 |
| `verdict.go:162` 改密未完成 | 读 `Error == ""` | **消失**，由 `passwd.give_up` 的触发条件接管 |

## 十一、测试断言：语义 / 措辞分类（文件级）

| 文件 | 命中分类名次数 | 性质 | 02 要做什么 |
| --- | --- | --- | --- |
| `internal/verdict/verdict_test.go` | 44 | **语义**（给结构 → 期望分类名） | 那批「给布尔 → 期望中文分类名」的表格**搬去纯函数测试**（§十）；判定层只留「给文本 → 期望分类名」 |
| `internal/verdict/keywords_test.go` | 9 | 语义（分类必须被配置登记） | 随常量退役重写；`TestCodeDefinedCategoriesAreDeclaredInConfig` 删除 |
| `internal/output/terminal_test.go` | 21 | 混合：统计块落点（语义）+ 字符串（措辞） | 落点断言保留；`TestPrintStatisticsCodeDefinedCategoryFollowsConfig` 删除 |
| `internal/output/reporter_test.go` | 15 | 混合（执行期日志 / 错误详情） | 措辞类期望值跟着改 |
| `internal/output/xlsx_test.go` | 2 | 措辞（分类列文本） | 跟着改 |
| `internal/config/passwd_test.go` | 6 | 语义（`code_defined` 校验） | `TestLoadPasswdConfig` / `TestUnmatchedCategoriesAreReportedNotFatal` 随字段删除重写 |
| `internal/result/statistics_test.go` | 1 | 语义（分类聚合） | 不动 |

**以中文错误文本作输入的用例**：全部在 `testdata/error_classification_cases.toml`，共 17 条，改造后换成报文输入、期望分类不变。涉及的分类：`权限不足`、`远程路径不存在`、`资源耗尽`、`密钥解析失败`、`无法建立会话`、`连接被断开`（6 条）、`远端没有文件`（2 条）、`SFTP失败`（2 条）、`本地文件错误`。

**语料的缺口**（spec 已指出，这里补实）：33 条里只有「部分成功」一条真的走结构字段；`任务已取消` 那条靠 `error = "context canceled"` 命中关键词，语料结构里**没有「被取消」字段**。02 要给语料补上：
- `canceled`、`connect_success` 字段（用于 `abort.canceled` / `abort.connect_canceled`）；
- `passwd` 相关字段（`passwd_fed_any`、`passwd_early_close`、`enter_signal_seen`）；
- 缺的形态：`trigger_miss`、`exhausted_timeout`、`no_prompt`、`no_signal`、`no_reply`、`give_up`、`not_expired`。

## 十二、开工前的两个提醒

1. **成功规则那条新增**拦的是「代填没送完但命令退出 0」——今天它靠 `verdict.go:94` 的一级分支抢在成功规则之前；一级分支删掉后，没有这条就会误判成功。回归测试必须覆盖它。
2. **报文措辞与配置关键词的耦合**没有任何机器保证：语料只喂手写文本，执行侧与判定层之间没有共同接缝。03–06 每摊都要跑一次真机集成测试（`//go:build integration`，见下），否则 07 之前发现不了「报文写错一个字 → 整类失败落进未归类」。

## 十三、集成测试的环境约定（04–06 用）

- 文件头 `//go:build integration`，默认 `go test ./...` 不加载。
- 连接信息从环境变量取：`SSHFLEET_TEST_HOST` / `SSHFLEET_TEST_PORT` / `SSHFLEET_TEST_USER` / `SSHFLEET_TEST_PASSWORD`；缺任何一项 `t.Skip`。
- **本机已验证可用**：WSL Ubuntu-26.04，`172.28.118.49:22`。账号：`sshfleet_ok`（普通，密码 `Probe#2026ok`）、`sshfleet_expired`（密码已过期）、`sshfleet_nologin`（nologin shell）。
- **这台 WSL 长期在后台运行，不要 `wsl --shutdown`。** 重启会让 NAT 地址变化，localhost 转发有好一阵不通。
- **用 IP 连，不要用 localhost**：`localhost:22` 时通时断，用 `172.28.118.49:22` 稳定。
- **待补**：`sshfleet_ok` 的 `NOPASSWD` sudo（现在一律加 `--no-sudo` 跑）；「远端不吭声」的畸形账号（改密 `no_prompt` 档）。

## 十四、实际落地与初始清单的偏差（02、03 实施后回填）

看这一节可以少走弯路——下面每一条都是实施时**只有动手才会暴露**的东西。

| # | 初始清单怎么写 | 实际怎么了 | 在哪 |
| --- | --- | --- | --- |
| 1 | 出厂报文 25 条（§九 框架表） | 核准并补全到 **38 条**（01 核准 36 条，02 实施时又补 `transfer.sudo_failed` 与 `transfer.remote_exists`） | §九 表下注 |
| 2 | `transfer.*` 逐条给环节名（13 处 `fmt.Errorf`） | 归成 3 条：`remote_file_failed` / `local_file_failed` / `tmp_failed`，原始错误跟在冒号后保留区分度 | 工单 02 Comments |
| 3 | 报文格式放「最底层的公共包」 | 实际放在 `internal/ssh`。放 `common` 会让 `ssh` 经 `common` 间接依赖 `log`；`ssh` 已被 `verdict`/`batch`/`output` 依赖，放这里环最浅 | 工单 02 Comments |
| 4 | 「部分成功」按普通条目挂报文即可 | **必须排到第一组最前面**：那一格里同时躺着逐文件失败原因，排在后面会被「权限不足」抢走。**后已退役**（2026-10-20，上传改失败即回滚，见第 10 条） | 工单 02 Comments |
| 5 | 「代填用尽后超时」已改报文 | 实地发现它还挂着旧中文关键词、且排在「触发词未命中」之后，**两处都错**，已修 | 工单 02 Comments |
| 6 | `run.rejected` 在命令通道生效 | **命令通道上走不到**：`NewSession` 先失败，已被 `session.create_failed` 接住。它实际只在代填通道的 `Start` 被拒时出现 | 工单 03 Comments |
| 7 | 退出码 0 也照旧拼「执行失败(退出码0)」 | 报文时代这条自相矛盾（报文已经说明了失败原因），改为落第一组显示原文 | 工单 02 Comments |
| 8 | 判据表里 `文件不可读` 那条（§七 表） | **对应的文本走 `fatal` 而不是 `Result.Error`，本就命中不了**。04 已定：**撤掉**，采信口径见 §七 表下注（开工前不归类、开工后保兜底，界线是「节点开没开工」） | §七 表下注 |
| 9 | 语料 33 条、17 条中文输入 | 实测 17 条对得上；`建立连接失败 - …` 是 **6 条**不是 7 条。语料经 02/03 扩到 47 条，审计后又补回滚与代填中止 **3 条**，终值 **50 条** | 工单 02/03 Comments、`fix-list.md` |
| 10 | `transfer.finished` 与「部分成功」按 02 定稿落地 | **同日退役**：上传改为「失败即回滚」（ADR-0022），传输只剩全成 / 全不成，`finished` 不再产出，接替者 `rolled_back` / `rollback_partial`；`SFTP失败` 按语义拆成六条；「执行超时」挪到两条代填分类之后（否则「触发词未命中」整类被抢走） | `docs/issues/2026-10-20-2245-上传失败回滚/`、工单 04/06 Comments |

**§十四第 10 条的补充**：ADR-0021 更新记录已点名「决定」第 1 条示例、第 8 条第 2 款、`spec.md`《预算：全部报文清单》里涉及 `transfer.finished` 的三处不再约束；本文件 §九 M29 亦有退役注记。
