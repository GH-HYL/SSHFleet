# 代码缺陷清点 2026-10-17 · 规格

Status: needs-triage

> **来源**：2026-10-17 核对全仓议题目录的「记录与事实」时，另行查出的代码层缺陷，集中登记于此，供开发者择期集中处理。
>
> **性质**：只登记，不含修复方案——每条只写「现象 + 证据 + 影响」，怎么修等开工时再定。
>
> **准入**：只收**已核实属实**的（有 file:line 或可复现证据）。未经核实的猜测不收。

---

## 问题陈述

使用者拿到的是「报错说得清、危险命令拦得住」这个承诺。本轮核对发现这份承诺有**七处**没兑现：危险命令的检查对非 shell 脚本既漏报又误报；`--yes` 下高危档提示不上屏；`keywords_passwd.conf` 留了一条永远匹配不上的关键词；「账号过期」的判据可能只在英文环境成立；还有两条报错文案没点名到选项。

另有三处「工单说做了、代码里找不到」：一个字段根本不存在、一个说是删掉的上限还在、一条工单 10 项只改完 8 项。这些不影响使用者，但让工单失去作为证据的价值——照它去核会核出反结论。

---

## 方案

**本议题只做登记，不承诺修复。** 修与不修、先修哪条，由开发者另行排期。

每条缺陷落地时，按需要拆成工单；缺陷严重度差异很大（**D1 高危**，D2/D4/D8 是对外承诺与实现不符，D5/D6/D7 只是工单记录失真，D9/D10 是低优先登记），不做「一次性全修」的假设。

---

## 使用者故事

1. 作为运维人员，我不想在非 shell 脚本里藏着的危险命令被放过去，这样批量执行时我不用担心漏掉一条 `rm -rf`。
2. 作为运维人员，我不想一批里因为一条**误判**的危险命令就整批跑不了，这样 2000 台的作业不必因为一条假警报全部作废。
3. 作为排查改密失败的运维人员，我想知道失败到底卡在哪一步，这样不用一台台登上去看。
4. 作为在中文系统上跑的运维人员，我想让「账号过期」这类判断在我机器上也成立，而不是只认英文提示。
5. 作为接手这个仓库的人，我不想照着工单去 `git show` 一个不存在的提交，这样我能真的看见当时的改动。

---

## 实现决策

本议题不产出实现。

---

## 测试决策

本议题不产出测试。

各条缺陷修复时的测试接缝另行说定。可参考的既有先例：
- `internal/verdict/verdict_test.go` 的 `TestErrorClassificationCorpus`（语料驱动的分类回归）
- `internal/dangercheck/dangercheck_test.go` 的 `testdata/dangerous_cases.tsv`（语料驱动）

---

## 不在本次范围

- 修复动作本身。本议题只登记现象与证据。
- D1 的处置方向（按解释器分别解析 / 保守拦截 / 声明能力边界）——待开发者选定。

---

## 补充说明

### 缺陷清单

> **现存有效缺陷 9 条**：D1、D2、D4、D5、D6、D7、D8、D9、D10。
> D3 已撤销（推断有误，见该条）；S4–S7 是设计边界，不算缺陷。**以下计数以本行为准。**
>
> 严重度：**高危**（使用者可感知且会致害）／**中**（对外承诺与实现不符）／**记录**（工单失真，不影响使用者）

#### D1 · 非 shell 脚本的危险内容既不检测、又误报阻断整批 —— **高危**

**现象**：危险命令检查用 **shell 语法**解析一切脚本内容，而解释器表里有 15 种解释器（`command.go:324-338`，含 `python` / `perl` / `ruby` / `node` / `php` 等非 shell 语言）。于是非 shell 脚本的内容被当成 shell 解析——解析不了的部分直接放过（漏报），而解析出结构的又被按 shell 语义判危险（误报）。

**证据**：
- `modules/SSHFleet_Go/internal/dangercheck/parse.go:12` — `mvdan.cc/sh/v3/syntax`（用 shell 语法解析一切内容，不分解释器）
- `modules/SSHFleet_Go/internal/ssh/command.go:324-338` — `interpreterTable` 的 15 个表项（表头在 323），其中非 shell 语言 8 项（`python` / `luajit` / `lua` / `node` / `nodejs` / `perl` / `ruby` / `php`）
- `modules/SSHFleet_Go/internal/ssh/command.go:397` — `InterpreterBaseName` 会剥掉版本号（`python3` → `python`），所以带版本号的名字同样落在表内

**影响**：漏报让危险命令溜过去；误报更严重——`forbidden` 档没有绕过手段，一条误判会让整批 2000 台一台都跑不了。

**复现**：`-c 'python3 -c "import os; os.system(\"rm -rf /tmp/target\")"' --yes`（漏报）；含 `rm -rf` 字样的无害 python 脚本走 `-s`（误报）。

> **本条的边界**：问题不在「表里有 15 种」这个数，而在「表里既然认了 8 种非 shell 语言，检测器却只认 shell 一种语法」。要确认某条具体命令是否真被误判，需按上列复现方式实跑。

#### D2 · `--yes` 下高危档危险命令不上屏（`forbidden` 档上屏） —— **中**

**现象**：两档行为不同。

- **`forbidden` 档**（如 `rm -rf /etc`）：`--yes` 下**照常上屏**，打印完整的警告框，然后退出。
- **高危 / 中 / 低档**（如 `rm -rf /var/log/app`）：`--yes` 下**不上屏**，提示只写进日志。

**证据**：
- `modules/SSHFleet_Go/main.go:302-304` — `forbidden` 档无条件调 `output.PrintDangerWarning(dangerReport, true)`，`true` 是字面量，与 `--yes` 无关
- `modules/SSHFleet_Go/main.go:308-312` — 非 `forbidden` 档仅在 `!in.Disinteractive` 时上屏
- `modules/SSHFleet_Go/main.go:315-319` — 这处的 `ConfirmReq` **没设 `SkipLine`**，只传 `Trace`
- `modules/SSHFleet_Go/internal/common/interactor.go:96` — `SkipLine` 字段就在：「非交互跳过时上屏的一行」。机制已经具备，`confirm.go:54` 与 `confirm.go:123` 两处已在用它，**危险命令这处没接**

**影响**：无人值守跑高危命令时，终端上只剩一句「[非交互模式] 跳过执行选项/参数确认环节」，**看不出放行了一条有风险的命令**。事后只能翻日志。

**说明**：`CHANGELOG.md` 里没有任何一条交代过这个差异，使用者无从知道 `--yes` 会让高危提示消失。

**实测**（2026-10-17，本机编译后跑，最小配置与清单，不连真机）：

```text
$ ./SSHFleet.exe -f nodes.csv -c "rm -rf /etc" --yes
╔════════════════════════════════════════════════════════╗
║                   ✕  发现禁止命令  ✕                   ║
...
║           此命令被禁止执行，程序将立即退出！           ║
╚════════════════════════════════════════════════════════╝

$ ./SSHFleet.exe -f nodes.csv -c "rm -rf /var/log/app" --yes
 [非交互模式] 跳过执行选项/参数确认环节，直接执行
【127.0.0.1】 连接: 失败 - 0.006s
...
```

后一条**终端上看不到任何危险提示**。

**处置方向未定**（可能只需补文档，不一定改代码）：

| 方向 | 做法 | 代价 |
| --- | --- | --- |
| 甲 | `--yes` 下高危档也上屏一行摘要 | 无人值守的屏幕输出会多出提示，可能被当噪声 |
| 乙 | 维持现状，在手册与 CHANGELOG 里写清「`--yes` 下高危提示进日志」 | 行为不变，但使用者至少知道去哪查 |
| 丙 | 给这处 `ConfirmReq` 补 `SkipLine`（机制已具备） | 改动最小，但要想清楚这一行写什么、会不会刷屏 |

**来源**：2026-10-13 安全分析报告的 S3（当时在 `docs/issues/security-audit/`，该目录已删，原文可按 `ce618f1` 取回）。

---

> ~~D3 · 「改密未完成」与「提示词没认出」两条分类走不到~~ —— **2026-10-17 撤销，本条推断有误**
>
> 原先的判断是「`catPasswdGiveUp` 的条件没排除 `catPasswdNoPrompt`，所以第二组永远轮不到」。
> **这是错的。** 四个改密失败分类各有独立前提：
>
> ```text
> catPasswdNoPrompt  = 一句都没喂出去、也没等到点
> catPasswdNoSignal  = 一句都没喂出去、等到点了
> catPasswdNoReply   = 喂进去了、等到点也没下文
> catPasswdGiveUp    = 喂进去了、远端还在重问而已给的答案用完
> ```
>
> `catPasswdGiveUp` 的前提是「喂进去了（`PasswdFedAny`）」，`catPasswdNoPrompt` 的前提是「没喂出去」——**两者互斥，第二条走得通**。
>
> 证伪方式：`go test ./internal/verdict/ -run TestJudge` 通过，其中 `verdict_test.go:211` 的用例正是「喂出去了、改密非 0 收场 → 改密未完成」，即第二组确实可达。判 `CHANGELOG` `[9.1.4]` 的四种情况与代码一致。
>
> 教训：只看了分支顺序、没看完整条件（`!res.PasswdFedAny`）就下了结论。

#### D4 · `keywords_passwd.conf` 有一条永远匹配不上的关键词 —— **中**

**现象**：该关键词对应的分类已改由结构事实直接给出，不再读远端文本，于是那条关键词永远匹配不上。

**证据**：
- `modules/SSHFleet_Go/config/keywords_passwd.conf:174` — `'密码未过期，跳过改密'`
- `modules/SSHFleet_Go/internal/verdict/verdict.go:41` — `catPasswdNotExpired` 定义
- `modules/SSHFleet_Go/internal/verdict/verdict.go:119` — 直接赋分类名，不查表

**影响**：分类结果照旧正确，但配置里留一条死条目，改配置的人会以为它还起作用。

**来源**：`findings-2026-10-13.md` 的 P1（该清单原文作为证据底本保留在本目录）。

#### D5 · `AuthFailure` 字段全仓不存在 —— **记录**

**现象**：工单称新增了 `ssh.Result` 的 `AuthFailure` 字段并配文案「密钥与密码均失败」，实际**代码里没有这个字段**（`verdict_test.go:145` 有个名为 `TestAuthFailureKeywordPath` 的测试，测的是关键词优先级，不依赖该字段；CHANGELOG 与 ADR 里出现的「密钥与密码均失败」是文案记录，不是字段）。

**证据**：
- **代码里没有 `AuthFailure` 这个字段**：`modules/SSHFleet_Go/internal/ssh/` 下 `Result` 结构体没有该字段
- `modules/SSHFleet_Go/internal/verdict/verdict_test.go:145/147` — 名为 `TestAuthFailureKeywordPath` 的测试存在，但它测的是关键词优先级，**不依赖该字段**
- `modules/SSHFleet_Go/internal/ssh/client.go:26`（定义）、`:121`（赋值）— 留下的相关变量是 `publicKeyOffered`（连接状态，与 `AuthFailure` 不是一回事；`client.go:24` 是 `banner` 字段）
- 全仓 `AuthFailure` 共 9 处命中，**全部是测试名或文档里的文案**，没有一处是字段声明或读写

**影响**：工单的 7 例验收表没有现存代码可以对应。要么字段后来被清理删掉，要么当初就没落地。

**说明**：本条原先写作「全仓 grep 零命中」，**不准确**——`AuthFailure` 作为**测试名**与**文档里的文案**是有命中的（`verdict_test.go`、`docs/adr/0004`、`docs/adr/0005`、`rewrite-to-go/spec.md`、`CHANGELOG.md` 等）。准确的说法是「**没有这个字段**」，不是「全仓零命中」。

**来源**：`docs/issues/rewrite-to-go/issues/16-auth-rules.md`。

#### D6 · 工单说删了 20 节点上限，代码里还在（且是有意保留的） —— **记录**

**现象**：`rewrite-to-go/issues/20-output-terminal.md` 明写「删掉 20 节点显示上限与排队机制」，代码里原样保留。

**证据**：
- `modules/SSHFleet_Go/internal/output/progress.go:39` — `maxVisibleNodes = 20`
- `modules/SSHFleet_Go/internal/output/progress.go:211-225` — 满 20 为止、其余排队
- `modules/SSHFleet_Go/internal/output/progress_test.go:69-89` — 仍在断言该上限

**影响**：**工单这一句是失真记录**。照工单去核的人会以为上限已经没了。

**不是缺陷**：`progress.go:211` 的注释写明这是**有意保留**的——「不按终端高度自适应（72 台满屏 0% 空条观感极差，**用户** 2026-09-15 裁定）」——「用户」是代码注释原话。`progress-update-lag/spec.md` 也把它记为「保留显示位管理」。

**所以本条要修的是工单的措辞，不是代码**。**不要照这条去删上限。**

**来源**：`docs/issues/rewrite-to-go/issues/20-output-terminal.md`。

#### D7 · 两条报错文案仍不含选项名 —— **记录**

**现象**：工单列了 10 条「通篇不含选项名」的文案，要逐条改成点名到 `-a`。前 8 条已改（`check.go:136/139/172`、`answer.go:91/101/104/107/116/124` 都带上了选项名），**这两条原样**——它们出自代填文件的行解析，使用者看到时不知道说的是 `-a`。

**证据**：
- `modules/SSHFleet_Go/internal/cli/answer.go:152` — `只有一列：缺触发词（第 1 列是代填内容，第 2 列起是触发词）`
- `modules/SSHFleet_Go/internal/cli/answer.go:156` — `触发词不能为空`

**影响**：使用者看到这两条不知道是哪个选项带进来的。

**说明**：这两条与其它 8 条有一点不同——它们的正文已把「第 1 列是代填内容、第 2 列起是触发词」解释清楚了，读者未必猜不到是代填。所以这条按**记录**登记，是否值得改成点名 `-a`，由开发者定。

**来源**：`docs/issues/usage-error-message-audit/issues/02-name-the-flag.md` 的待改清单。

#### D8 · 「账号过期」判据可能只在英文环境成立 —— **中**

**现象**：该分类的判据只配了英文提示，理由写的是「sshd 不翻译自己的消息」。但客户端的认证请求里 `Language` 字段从不赋值，等于交给服务端按默认 locale 决定。

**证据**：
- `modules/SSHFleet_Go/config/keywords_error.conf` — 「账号过期」只有英文判据
- `golang.org/x/crypto@v0.57.0/ssh/client_auth.go:577` — `initiateMsg` 的 `Language` 从不赋值

**影响**：`zh_CN` 系统上该提示可能被译成别的措辞，落进兜底分类。

**说明**：**本条待实测确认。**既有真机记录（`docs/issues/error-classification/spec.md` §八，11 场）是在英文环境跑通的，不能仅凭代码断定中文环境必错。要确认得真机在 `zh_CN` 账号上跑一次 `chage -E`。

#### D9 · `common.Interactor` 的 `Prompt` / `PromptPassword` 无包外调用者 —— **记录**

**现象**：两个导出方法都没有包外调用点。`Interactor` 本体是在用的（`Confirm`、`Disinteractive`、`NewInteractor` 均被正常调用），所以不是整块死代码，只有这两个方法悬空。

**证据**：
- `modules/SSHFleet_Go/internal/common/interactor.go:77-89` — `PromptPassword`，其中 `:88` 内部调 `Prompt`
- 包外搜 `.Prompt(` / `.PromptPassword(` 只命中 `interactor.go` 自身

**影响**：低。注释自陈「保留供日后改回来」，**可能是有意保留**。清理前需先确认当初是否有意留用——这一点需开发者判定，不属自动成立的缺陷。

**来源**：`findings-2026-10-13.md` 的 P2。

#### D10 · `batch` 与 `log` 两个包没有测试 —— **记录**

**现象**：`internal/batch`、`internal/log` 两个包测试文件数为 0。（原先还有 `internal/confirm`，已于 2026-10-15 补上测试，提交 `469630d`，故不在本条范围。）

**证据**：
- `internal/batch` — 并发执行 + 进度聚合，0 测试
- `internal/log` — 日志，0 测试

**影响**：`batch` 是执行主干的一环（并发跑节点、聚合进度），无测试意味着改动它没有回归网。

**说明**：**这项该不该做取决于开发者口径，不是自动成立的。** ADR-0012 明确「架构审查不以可测性为收益依据」——本条登记的是**事实缺口**，不是建议补测试。

**来源**：`findings-2026-10-13.md` 的 T1。

---

### 设计边界（2026-10-13 安全分析的 S4–S7）

> **为什么单列**：这四条不是缺陷，是「清楚了但选择接受」的边界。2026-10-17 删掉安全分析目录时它们曾一并丢失，经审计指出后从 `ce618f1` 取回。留在这里的理由是——**它们是下次改动会踩到的边界**，不记下来就会被当成遗漏重查一遍。

#### S4 · 危险检测只覆盖目标脚本文件本身

检测器只读 `-s` 指向的**那一个文件**。脚本内部的 `.` / `source` 引入的其他文件、运行期 `curl | bash` 拉取的内容，**均不在检测范围内**。

这是静态检测的固有边界（不做动态追踪就无法覆盖），但要明确：**「检测过」不等于「整条执行链都看过」**。

#### S5 · 代填值与改密回显的保护不一致

两条路径同样是 PTY、同样开回显、同样把输出写进 `result.Output` 并落盘归档，但**只有改密路径做了脱敏**：批量改密有 `scrubPasswdSecrets` 把凭据擦成 `****`，交互代填则直接取原文。

**代填的设计用途是回答「选架构 / 选 deb 或 rpm」这类选项，不是承载密码**，故不构成缺陷，仅记录落差。开发者提出过后期设想（是否实现未定）：增加「代填内容是否脱敏」的配置开关，默认不脱敏。

#### S6 · `assets/` 清单脱敏依赖 IPv4 假设

归档时按 `common.IsIPv4Literal(fields[0])` 判断该行是否脱敏；判断不通过的（表头、注释、异常行）**原样写出**。也就是说清单第一列不是 IPv4 时，脱敏静默失效——不报错、不提示。

#### S7 · 主密钥以 0644 写入 rc 文件

主密钥明文写入 `~/.bashrc`（或 `~/.zshrc`），权限 `0o644`。实际安全性由 `~` 目录权限（通常 0755）兜底，属「恰好安全」。

**本工具面向运维人员，不对其本机操作失误设防**，故不按风险计。

---

## Comments

- 2026-10-17：建立。登记本轮核对全仓议题目录时查出的代码缺陷，其中 D1/D2 高危、D3/D4/D8 为对外承诺与实现不符、D5/D6/D7 为工单记录失真、D9/D10 为低优先登记。全部只登记、未修。
- 2026-10-17：**D1/D2 的来历**。这两条源自 2026-10-13 的安全分析报告（当时在 `docs/issues/security-audit/`，S1/S2/S3），该目录已于同日按开发者指示删除。报告的原始提交是 `ce618f1`（`docs(安全审计): 归档静态分析、实测验证与汇总报告`），需要复原时用 `git show ce618f1` 取回。删目录时没有把这一点写清楚，等于让这两条缺陷失去出处——此处补上。
- 2026-10-17：并入原 `backlog-2026-10-13/findings.md` 的 P2（→ D9）与 T1（→ D10，其中 confirm 包已于 `469630d` 补上测试，故排除）。该清单的 P1 与本议题的 D4 是同一条。**清单原文已保留为 `findings-2026-10-13.md`**（目录撤销时曾一并删除，经审计指出后取回）。
- 2026-10-17（同日，开发者指示）：security-audit/ 整个议题已删除（历史问题不再追溯）。本议题的 D1/D2 保留并改为自带证据与复现方式——D2 的实测结论是**两个档位行为不同**：forbidden 档在 --yes 下照常上屏，高危 / 中 / 低档只进日志不上屏。
