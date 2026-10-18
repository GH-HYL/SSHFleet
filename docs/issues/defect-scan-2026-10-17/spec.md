# 代码缺陷清点 2026-10-17 · 规格

Status: needs-triage

> 2026-10-18：本议题的登记与分诊已完成，D2/D4/D7/D9 四张工单已实施完毕，D8 已修，D5/D6 已在原工单补订正，D10 定为 wontfix，D1 移交重构议题。剩下 D1 的修复在 `docs/issues/dangercheck-text-regex/` 跟踪。

> **来源**：2026-10-17 核对全仓议题目录的「记录与事实」时，另行查出的代码层缺陷，集中登记于此，供开发者择期集中处理。
>
> **性质**：只登记，不含修复方案——每条只写「现象 + 证据 + 影响」，怎么修等开工时再定。
>
> **准入**：只收**已核实属实**的（有 file:line 或可复现证据）。未经核实的猜测不收。

---

## 问题陈述

使用者拿到的是「报错说得清、危险命令拦得住」这个承诺。本轮核对发现这份承诺有**七处**没兑现：危险命令的检查对非 shell 脚本漏报（D1）；`--yes` 下高危档提示不上屏（D2）；`keywords_passwd.conf` 留了一条永远匹配不上的关键词（D4）；「账号过期」的判据可能只在英文环境成立（D8）；还有两条报错文案没点名到选项（D7）。

> 2026-10-18 补记：D11（代码字面量被判危险）经开发者裁定属**有意取舍**，已撤出此清单，改列设计边界与 ADR-0019。

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
- D1 的修复（方向见 ADR-0019）——**已移交** `docs/issues/dangercheck-text-regex/`，本议题不再跟。
- D2 的实现（方向已定为甲，见该条）；本轮只做核实与登记。

---

## 补充说明

### 缺陷清单

> **现行有效缺陷 9 条**：D1、D2、D4、D5、D6、D7、D8、D9、D10（D1 的修复已移交，本议题只留现象与证据）。
> D1 的**修复**已移交给 `docs/issues/dangercheck-text-regex/`（重构型改动，单独立项）；本条只留现象与证据。
> D3 已撤销（推断有误，见该条）；D11 已改记为设计边界（2026-10-18 由开发者定为「误伤可接受」，见下「设计边界」一节与 ADR-0019）；S4–S7 是设计边界，不算缺陷。**以下计数以本行为准。**
>
> 2026-10-18 更新：D2 的处置方向已定为「`--yes` 与交互显示逐字一致」，尚未动代码。
>
> 严重度：**高危**（使用者可感知且会致害）／**中**（对外承诺与实现不符）／**记录**（工单失真，不影响使用者）

在下方，用一行加一张表把每条的去向说清，避免翻正文找。

#### 去向

| 条 | 去向 |
| --- | --- |
| D1 | 修复移交 `docs/issues/dangercheck-text-regex/`（重构，单独立项） |
| D2 | 工单 `issues/02-yes-shows-danger-warning.md` ✅ done |
| D4 | 工单 `issues/01-passwd-not-expired-keyword.md` ✅ done |
| D5 | 不改代码：已在 `rewrite-to-go/issues/16-auth-rules.md` 补记录订正 |
| D6 | 不改代码：已在 `rewrite-to-go/issues/20-output-terminal.md` 补记录订正 |
| D7 | 工单 `issues/03-answer-row-errors-name-the-column.md` ✅ done |
| D8 | 已修（中文判据 + 语料），见 CHANGELOG `[9.1.7]` |
| D9 | 工单 `issues/04-drop-unused-interactor-prompts.md` ✅ done |
| D10 | wontfix：不预先补测试，用到再说 |
| D11 | 设计边界（ADR-0019） |

---

> **2026-10-18：已移交**，处置方向定为「判定引擎改为纯文本正则」，属重构，单独立项实施——见 `docs/issues/dangercheck-text-regex/spec.md` 与 ADR-0019。**本议题不再跟 D1 的修复**，下方保留现象与证据作出处。

#### D1 · 非 shell 脚本的危险内容既不检测、又误报阻断整批 —— **高危**（已移交）

**现象**：危险命令检查用 **shell 语法**解析一切脚本内容，而解释器表里有 15 种解释器（`command.go:324-338`，含 `python` / `perl` / `ruby` / `node` / `php` 等非 shell 语言）。非 shell 内容被当成 shell 解析，结果是两半都坏：**该拦的放过（漏报）**，**不该拦的被判禁止（误报）**。

**证据**：
- `modules/SSHFleet_Go/internal/dangercheck/parse.go:12` — `mvdan.cc/sh/v3/syntax`（用 shell 语法解析一切内容，不分解释器）
- `modules/SSHFleet_Go/internal/ssh/command.go:324-338` — `interpreterTable` 的 15 个表项（表头在 323），其中非 shell 语言 8 项（`python` / `luajit` / `lua` / `node` / `nodejs` / `perl` / `ruby` / `php`）
- `modules/SSHFleet_Go/internal/ssh/command.go:397` — `InterpreterBaseName` 会剥掉版本号（`python3` → `python`），所以带版本号的名字同样落在表内
- `modules/SSHFleet_Go/internal/dangercheck/check.go:17-35` — `Check` 只取 `-c` 的命令原文或 `-s` 脚本的**文件全文**交给 `Analyze`，**不拿解释器**（`args` 里有，但这里没用），所以「按哪种语言解析」根本无从谈起
- `modules/SSHFleet_Go/internal/dangercheck/parse.go:43-49` — `ParseSource` 先整份解析，**成功就只走语法树**（`parseSource`，`:52-62`），失败才 `parseFallback` 逐行回退（`:305-332`）

**根因**（2026-10-18 实跑坐实，**已复核到机制层面**；没有直接调用 `Analyze` 的探针验证，见下）：判定分成两条路，走哪条只取决于「整份文本在 shell 语法下能不能解析通过」，与脚本实际是什么语言无关——

```text
整份能解析  → 只看语法树（parseSource，parse.go:52-62）
              python 的 def f(): 被当成 shell 函数头，后面的语句挂进函数体、
              不进命令段 → 里面的 rm -rf /etc 漏报
整份解析失败 → 逐行回退（parseFallback，parse.go:305-332）
              这一路按行再看，反而捞得到文本字面量 → 误报（见 D11）
```

同一份 python 脚本「有时拦、有时不拦」，就是这个分叉造成的，不是随机。

> 复核依据：借助 `mvdan.cc/sh` 在本地模块缓存里的源码（`syntax/parser.go`）确认了几条关键行为——用 `def f():` 开头的文本能解析成功（`def f():` 被当 shell 函数定义）、含 `print(...)` 的文本会报 `` `foo(` must be followed by `)` `` 而失败。**没有**用探针直接调 `Analyze` 验证，原因是 `sshfleet/internal/*` 受 Go 的 internal 包规则保护，沙盒里的探针进不去。要在修法阶段用到这些段，先补探针。

**影响**：
- **漏报**：非 shell 里的危险命令整条溜过去，检测等于没做。
- **误报**：`forbidden` 档没有绕过手段，一条误判会让整批 2000 台一台都跑不了。

**复现**（2026-10-18，本机编译后跑，最小配置与清单，不连真机）：

```text
$ ./SSHFleet.exe -f nodes.csv -c 'python3 -c "import os; os.system(''rm -rf /etc'')"' --yes   → 未拦
$ ./SSHFleet.exe -f nodes.csv -c 'perl -e "system(''rm -rf /etc'')"' --yes                      → 未拦
$ ./SSHFleet.exe -f nodes.csv -c 'node -e "require(''child_process'').execSync(''rm -rf /etc'')"' --yes → 未拦
$ ./SSHFleet.exe -f nodes.csv -c 'bash -c "rm -rf /etc"' --yes                                 → 拦下（对照）
```

误报那一半见 D11（同一根因的另一面，单独登记）。

> **本条的边界**：问题不在「表里有 15 种」这个数，而在「表里既然认了 8 种非 shell 语言，检测器却只认 shell 一种语法」。

#### D2 · `--yes` 下高危档危险命令不上屏（`forbidden` 档上屏） —— **中**

**现象**：两档行为不同。

- **`forbidden` 档**（如 `rm -rf /etc`）：`--yes` 下**照常上屏**，打印完整的警告框，然后退出。
- **高危 / 中 / 低档**（如 `rm -rf /var/log/app`）：`--yes` 下**不上屏**，提示只写进日志。

**证据**：
- `modules/SSHFleet_Go/main.go:302-304` — `forbidden` 档无条件调 `output.PrintDangerWarning(dangerReport, true)`，`true` 是字面量，与 `--yes` 无关
- `modules/SSHFleet_Go/main.go:308-312` — 非 `forbidden` 档仅在 `!in.Disinteractive` 时上屏（`--yes` 把这一路整个跳过）
- `modules/SSHFleet_Go/main.go:315-319` — 这处的 `ConfirmReq` **没设 `SkipLine`**，只传 `Trace`
- `modules/SSHFleet_Go/internal/common/interactor.go:96` — `SkipLine` 字段就在：「非交互跳过时上屏的一行」。机制已经具备，`confirm.go:54` 与 `confirm.go:123` 两处已在用它，**危险命令这处没接**

> 上面后两条是 10-17 按「甲 / 乙 / 丙」三选一记的；方向定为甲之后，`SkipLine` 不再是主修法，仅作备选留档。

**影响**：无人值守跑高危命令时，终端上只剩一句「[非交互模式] 跳过执行选项/参数确认环节」，**看不出放行了一条有风险的命令**。事后只能翻日志。

**说明**：`CHANGELOG.md` 里没有任何一条交代过这个差异，使用者无从知道 `--yes` 会让高危提示消失。

**实测**（2026-10-17 首测；2026-10-18 复测，结论一致）：

```text
$ ./SSHFleet.exe -f nodes.csv -c "rm -rf /etc" --yes
╔════════════════════════════════════════════════════════╗
║                   ✕  发现禁止命令  ✕                   ║
...
║           此命令被禁止执行，程序将立即退出！           ║
╚════════════════════════════════════════════════════════╝

$ ./SSHFleet.exe -f nodes.csv -c "rm -rf /var/log/app" --yes     # exit=0
 [非交互模式] 跳过执行选项/参数确认环节，直接执行

【127.0.0.1】 连接: 失败 - 0.005s
...
```

后一条**终端上看不到任何危险提示**，只在工具日志里留下：

```text
[WARNING] 非交互模式放行危险命令（最高级别 high，分类 递归强制删除，来源行 1）：rm -rf /var/log/app
```

**处置方向已定：甲——`--yes` 下高危 / 中 / 低档也把完整的危险警告框打出来**（2026-10-18 开发者选定）。

- **口径（2026-10-18 开发者原话）**：`--yes` 不改变任何显示——屏幕上打什么、日志里记什么，与交互时**一模一样**；`--yes` 只是替人回答了一个「y」，跳过等待键盘输入这一步，其余逻辑与交互完全一致。
- 依据：虽然是非交互跑，但「那些内容该显示还是显示」，不能被 `--yes` 抹掉；警告框已经渲染好了（`output.PrintDangerWarning`），只是被 `!in.Disinteractive` 这道条件挡住了。
- 定案之前核查过同一个疑问：是不是 `Confirm` 这个 yes/no 函数把提示吞了。**不是**。`--yes` 下 `Confirm` 会照 Trace 留痕、照 `SkipLine` 上屏（`interactor.go:107-114`），`confirm.go:54` 与 `:123` 两处已在用它；这里是调用方在 upstream 就跳过了 `PrintDangerWarning`，`Confirm` 根本收不到任何要显示的内容。
- 落地时的改动面：`main.go:310-312` 去掉 `!in.Disinteractive` 这层包裹（`main.go:309` 的注释同步改），改成不管交互与否都调一次 `output.PrintDangerWarning(dangerReport, false)`；`CHANGELOG` 开当天段记一条。
- 验收：同一条命令在 `--yes` 与交互下，警告框与日志**逐字一致**；`--yes` 只是不等键盘。

<details><summary>曾考虑过的备选（未采用，留档备查）</summary>

| 方向 | 做法 | 为什么不选 |
| --- | --- | --- |
| 乙 | 维持现状，在手册与 CHANGELOG 写清「`--yes` 下高危提示进日志」 | 行为不变，无人值守时屏幕上看不出放行了一条有风险的命令 |
| 丙 | 给这处 `ConfirmReq` 补 `SkipLine`，`--yes` 下上屏一行摘要 | 信息量不如完整警告框，且要求另外定一行措辞 |

</details>

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

**2026-10-18 裁定：wontfix**——开发者定「不预先补测试，用到的时候再说」。本条从此**不再跟**，不建工单。以后要动 `batch` / `log` 时按当时的实际需要补，不做提前的全面健全。

---

#### 后续补充：D8 的改密侧判据

D8 的「中英两套」在第一轮只补了错误分类那一侧。改密失败分类走的是另一张表（`keywords_passwd.conf` 的分类会被插进同一张判据表的最前面），而那里没有「账号过期」。**已补**：改密配置新增「账号过期」分类，中英两条判据；语料补了改密模式的中英各一条，并把测试的判据表装配改成与 `main` 一致（加载改密分类并前置合并）——原先测试没合并这一步，改密路径的用例其实一直在用通用表。

**来源**：`findings-2026-10-13.md` 的 T1。

> ~~D11 · 危险规则会命中代码里的字符串与注释，误判为禁止~~ —— **2026-10-18 改记为设计边界，从缺陷清单撤下**
>
> 原先登记为缺陷，理由是 `print("reboot")` 这类代码字面量被判禁止、整批退出，属误报。
>
> **开发者 2026-10-18 裁定：误伤可接受**——「`print reboot` 就算危险行为，没事儿你打印它干啥？误伤就误伤了」。危险检测按原始文本做正则匹配，字符串与参数里的字面量同样参与判定；命中 `forbidden` 档即退出，不区分命中来源。决策见 ADR-0019。
>
> 这一条因此**不再当缺陷报**，改列在下方「设计边界」一节（保留 D11 编号，便于引用）。原复现记录如下，留作出处：
>
> ```text
> deploy.py：
>     import subprocess
>     # 清理构建产物
>     subprocess.run(["rm", "-rf", "/opt/build-cache"])
>     print("reboot the target")
>
> $ ./SSHFleet.exe -f nodes.csv -s deploy.py --yes
> ║    来源: 脚本: deploy.py   行号: 4   内容: print(reboot the target)   规则: 系统重启   级别: 禁止    ║
> ║           此命令被禁止执行，程序将立即退出！           ║
> ```

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

#### D11 · 危险检测按原始文本匹配，代码字面量同样参与判定（ADR-0019）

> 编号沿用缺陷清单里的 D11；2026-10-18 起从缺陷撤下、列为边界。

危险检测对 `-c` 的命令原文、`-s` 的脚本全文做**正则匹配**，匹配前去掉注释（整行、行尾、纯空白行），**其余内容全部参与**——包括代码字符串与参数里的字面量。命中 `forbidden` 档即打印警告框并以 1 退出，不区分命中来源。

**已知会误伤**：`print("reboot")`、`echo "won't rm -rf anything"` 这类代码里的说法会被拦下。**这是知情的取舍**（ADR-0019），不要当缺陷报。之所以接受：`forbidden` 档放过一条真危险命令的代价，高于误伤一条「没事去打印 reboot」的脚本。

---

## Comments

- 2026-10-17：建立。登记本轮核对全仓议题目录时查出的代码缺陷，其中 D1/D2 高危、D3/D4/D8 为对外承诺与实现不符、D5/D6/D7 为工单记录失真、D9/D10 为低优先登记。全部只登记、未修。
- 2026-10-17：**D1/D2 的来历**。这两条源自 2026-10-13 的安全分析报告（当时在 `docs/issues/security-audit/`，S1/S2/S3），该目录已于同日按开发者指示删除。报告的原始提交是 `ce618f1`（`docs(安全审计): 归档静态分析、实测验证与汇总报告`），需要复原时用 `git show ce618f1` 取回。删目录时没有把这一点写清楚，等于让这两条缺陷失去出处——此处补上。
- 2026-10-17：并入原 `backlog-2026-10-13/findings.md` 的 P2（→ D9）与 T1（→ D10，其中 confirm 包已于 `469630d` 补上测试，故排除）。该清单的 P1 与本议题的 D4 是同一条。**清单原文已保留为 `findings-2026-10-13.md`**（目录撤销时曾一并删除，经审计指出后取回）。
- 2026-10-17（同日，开发者指示）：security-audit/ 整个议题已删除（历史问题不再追溯）。本议题的 D1/D2 保留并改为自带证据与复现方式——D2 的实测结论是**两个档位行为不同**：forbidden 档在 --yes 下照常上屏，高危 / 中 / 低档只进日志不上屏。
- 2026-10-18：**开发者指示核实本议题**。D2 复测成立（结论与 10-17 一致，见该条实测）；D1 实跑坐实了漏报（`python3 -c` / `perl -e` / `node -e` 里的 `rm -rf /etc` 全部未拦，`bash -c` 对照能拦），并按 `parse.go:43-49` 与本地模块缓存里 `mvdan.cc/sh` 的解析行为复核出根因——「整份能解析就只走语法树、失败才逐行回退」这个分叉。**新增 D11**：同一分叉的另一面，危险规则会命中代码里的字符串与注释，`print("reboot")` 就被判禁止、整批退出。
- 2026-10-18：**D2 处置方向由开发者选定为「甲」**——`--yes` 下高危 / 中 / 低档也打出完整的危险警告框。定案前核查过「是不是 `Confirm` 把提示吞了」，结论是否：`Confirm` 在 `--yes` 下会照 `Trace` / `SkipLine` 处理，这里的问题在调用方提前跳过了 `PrintDangerWarning`（`main.go:310-312` 的 `!in.Disinteractive`）。本轮**未改代码**，等开工授权。
- 2026-10-18：**探针没跑通，如实记录**。为直接验证 D1/D11 的根因，在 `build/atest-d2/probe3` 建过一个直接调 `dangercheck.Analyze` 的探针，被 Go 的 internal 包规则挡住：`use of internal package sshfleet/internal/dangercheck not allowed`。本轮**没有**「已跑过、能当场重现根因」的探针命令；D1/D11 的复现手段是上面两处的可执行命令（验证现象，不验证根因）。将来的修复若依赖根因，**先把探针补上再改代码**（红线 5）。
- 2026-10-18（开发者经拷问定案）：**D1 的处置方向改为「按原始文本做正则匹配」**，决策落 ADR-0019。要点：判定对象是命令 / 脚本的原始内容，不管下发时怎么包裹；匹配前只去掉注释，字符串与参数里的字面量照常参与；`-c` 与 `-s` 共用一套匹配器，不再按解释器分语言解析。
- 2026-10-18（同轮定案）：**D11 撤出缺陷清单，改列设计边界**。开发者原话「`print reboot` 就算危险行为，没事儿你打印它干啥？误伤就误伤了」——命中来源不构成豁免。缺陷计数据此从 10 条改为 9 条。
- 2026-10-18（同轮定案）：**D2 的处置方向定为「`--yes` 与交互显示逐字一致」**——`--yes` 只是替人回答「y」，屏幕上打什么、日志里记什么与交互时一模一样，不因 `--yes` 少打警告框。
- 2026-10-18（同轮定案）：**D1 的修复单独立项**，落 `docs/issues/dangercheck-text-regex/spec.md`。判定引擎要从「按语言解析」整体换成「纯文本正则」，属重构且需要大量语料测试，不适合并进本缺陷清单议题；本议题的 D1 转为只留现象与证据、标记已移交。
