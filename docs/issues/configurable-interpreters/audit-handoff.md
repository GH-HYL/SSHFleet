# 审计交接：解释器可配置 → 进度条直出 → 中断分类（一批）

Status: ready-for-human
Type: task

给接手审计的会话：对**下面这批改动**做一次独立审计。**先读工作区里的代码、文档与提交再下结论**，不要只凭这份稿子。

> 这篇住在 `configurable-interpreters/` 是因为它最初只交接解释器那批；现在把后两批（进度条、中断分类）一并收进来——它们是同一段连续工作的产物。路径没跟着换，换路径会打断既有引用。

---

## 审计范围（提交清单，从早到晚）

| 提交 | 一句话 |
| --- | --- |
| `b6e545f` | 脚本模式**首次补回 `$0`**（用 `exec -a`）；同日另含"精简传输明细""配置路径报错分形态"等 |
| `0d55746` | **解释器改为配置驱动**（`[interpreter]` 后缀→解释器映射），下发外壳由 `bash -lc` 改 **`sh -c`** |
| `8322aed` | 解释器配置的文档补充（参数屏解释器行、`--yes` 回退、不依赖 bash、匹配细节） |
| `ae5f5e7` | **`-a` 按解释器适配「程序文本参数」**（`-c`/`-e`/`-r`），范围外当场报错 |
| `cac636a` | **`--no-shell` 与自定义解释器互斥**；`$0` 补名改用 **`. /dev/stdin`** |
| `a64f162` | 命令行选项 **`--no-bash` 改名为 `--no-shell`**（连带字段、日志形态、文档） |
| `da9d06d` | 版本抬号 `8.2.0 → 9.0.0`（不兼容档，段内含两处不兼容） |
| `cc078a2` | 修掉 **6 个预存在的 gofmt 脏文件**——`gofmt -l` 现在**全仓干净** |
| `7a73836` | **进度条改直出**（去掉弹簧动画，不再冻结）；**中断按阶段分「建连被取消」/「任务已取消」** |

**意图记录（权威）**：`docs/adr/0014-configurable-interpreters.md`（解释器）；`docs/adr/0010-deliver-command-body-in-argv.md` 的《补充（2026-10-07）》（正文交付通道）。**后两批没有新 ADR**——进度条是缺陷修复，中断分类的结论落在 `docs/issues/progress-update-lag/issues/01-progress-bar-lags-behind-detail-lines.md` 与 `config/keywords_error.conf` 的注释里。

**已知遗留**（**不是缺陷**）：`docs/issues/configurable-interpreters/open-questions.md`（已 resolved，留结论与实测）。

---

## 建议的审计角度

1. **配置解析与校验**（`config/SSHFleet.conf` 的 `[interpreter]`、`internal/config/config.go`）
   - 校验是否完备；报错是否"站在用户角度"（判据见 `docs/个人开发规范.md` §五）。
   - "所有字段必填"的既定口径有没有被破坏；老配置缺段时报得是否清楚。
   - 后缀键归一化（转小写）与匹配（取最后一段扩展名、忽略大小写）是否一致、有无漏网。
2. **解释器解析**（`internal/cli/interpreter.go`、`args.go` 的 `Args.Interpreter`）
   - 与既有 `Args.Match`（同一手法：Parse 里从配置推导、执行侧只读 Args）是否一致；有没有第二处真相。
   - 回退询问（`ConfirmInterpreterFallback`）的时机与"答 n 即退出"是否符合 ADR-0014 第 4 条。
3. **命令拼装**（`internal/ssh/command.go`、`internal/ssh/interactive.go`）
   - **`sh -c` 外壳 + 显式 `envPrefix`（locale + PATH）**是否自洽、是否真的不依赖目标机有 bash。
   - **`ProgramTextArg`**（`-c`/`-e`/`-r` 表）与 `InteractiveCommand` 的取用是否一致；表外兜底是否可达（评审口径见规范 §二）。
   - **`$0` 补名**：命令模式的 `-c '<正文>' '<名字>'` 与"正文经 stdin"的 `. /dev/stdin` 两条路，行为是否与"脚本落到磁盘执行"一致；`scriptNameArg` 只给 shell 家族补，是否与 `shellFamilyNames` 同步。
4. **`--no-shell` 互斥**（`internal/cli/check.go` + `isBashInterpreter`）
   - 触发条件是否恰当（解释器是 `bash` 时不拦）；报错是否讲清了"为什么"。
   - 改名是否彻底：全仓 `grep -rn "no-bash\|NoBash\|no_bash"` 应**只剩** `args_test.go` 里那条"旧名不再出现在 help"的用例，与 `CHANGELOG` 的历史改名记录。
5. **范围声明**：ADR-0014 说"上传 / 下载 / 改密不受影响"——去代码核实（`internal/ssh/sftp.go` 的预检查走 `runCommand`，不经可配解释器）。
6. **文档一致性**：`README.md` / `docs/使用手册.md` / `docs/配置手册.md` / `docs/常见问题.md` / `CHANGELOG.md` 与代码是否一致（尤其 `--no-shell` 的措辞、"shell 指什么"的解释、`-a` 的解释器范围、中断两档分类的说法）。
7. **进度条渲染**（`internal/output/progress.go` 的 `newBar` / `barView` / `syncBars` / `Update`）
   - 核实"显示值恒等于真实进度"这件事在代码上是**构造性成立**的（`ViewAs(target)` + `progress()` 单调），而不是靠时序侥幸。
   - 有没有残留的动画机制（`SetPercent` / `FrameMsg` / `final` 标记 / 每节点动画模型）——应当一处不剩。
   - 回归口径：`TestDisplayTracksProgressUnderBurst`（1ms 灌 300 份快照，显示值不得落后）是否真能守住；它靠墙钟，评估是否够稳。
8. **中断分类**（`internal/ssh/exec.go` 的 `connectFor`、`internal/verdict/verdict.go` 的 `Judge`、`config/keywords_error.conf`）
   - `connectFor` 用 `ctx.Err() != nil` 判"这次建连失败是中断造成的"——核实其**误判面**（真正因目标机原因失败、恰好同时被中断的情形），以及为何比辨认 `operation was canceled` 文案更稳。
   - `Judge` 里那条规则的**位置**：为什么放在各模式的一级规则之后、二级匹配之前？重点核实它会不会盖掉更具体的一级结论（`密码过期` / `部分成功` / `密码未过期`），以及两侧的测试是否把这条边界钉住了。
   - `IsFallbackCategory` 的口径：结构性产出的分类名必须能被 `kw.Has` 认出，否则会当"报错原文"单排一行——核实新名字已登记、`TestCanceledSplitsByPhase` 是否覆盖。
9. **回归**：`go build ./... && go vet ./... && go test ./internal/...`；**`gofmt -l` 现在应为空**（`cc078a2` 已把那 6 个预存在脏文件清掉——旧稿里"应只剩 6 个"那句已失效）。

---

## 真机验证记录（供复核，不必重跑）

测试节点为本地 WSL2 上的 Ubuntu（**凭据不落任何文档**），非 root 账号；同一台机上跑过：

| 用例 | 结果 |
| --- | --- |
| `-c "echo ..."`（默认外壳） | 成功（此前失败过一次，是**环境**问题：配置默认 `sudo` 而该账号无权，非代码问题） |
| `-c ... --no-shell` | 成功（原样下发） |
| `-s probe.sh`（`.sh→bash`） | `SELF-0=[probe.sh]`、`PATH-HEAD=[/usr/local/sbin]`、`LC=[en_US.UTF-8]` |
| `.sh→sh` 后 `-s probe.sh` | `SELF-0=[probe.sh]`（**`. /dev/stdin` 在 `sh` 下也补 `$0`**） |
| `-s ... -a "1,请选择架构"` | 代填成功 |
| `.pl→perl` 后 `-s probe.pl -a ...` | 代填成功（**`perl -e` 生效**） |
| `--no-shell` + `command=perl` | 按预期**报互斥**，文案完整 |
| **进度条·爆发**（1000 台指向秒拒端口，pty 里录渲染流） | 录到 **415 个不同的百分比取值**，从 `0.0%` 平滑走到 `100.0%`（修复前同场景恒为 `0.0%`） |
| **中断·执行阶段**（4 台真节点跑 `sleep 20`，2 秒时中断） | 2 台「任务已取消」、2 台未执行；汇总 `任务已取消 ×2` 聚合成一行并带提示 |
| **中断·连接阶段**（20 台挂在拨号 + 4 台真节点，2 秒时中断） | 汇总 `建连被取消 ×8  任务已取消 ×1`——**修复前是 8 行缩进的报错原文、每行带一个 IP** |

**形态级实测**（各解释器的"程序文本参数"）：`bash`/`zsh`/`ksh -c`、`python3 -c`、`perl`/`ruby`/`node`/`lua -e`、`php -r` 均可执行程序文本；`perl -c`、`node -c` 不对（印证"非 shell/python 不能用 `-c`"）。

> 中断验证用的是 **SIGTERM**（非交互 shell 的后台作业会把 SIGINT 设成忽略）：两者走的是同一个 `signal.NotifyContext` 分支，`main.go` 的 ctx 只由信号取消、无 deadline。

---

## 已知、非缺陷（别当 bug 报）

- **老配置必须补 `[interpreter]` 段**：不兼容是**有意**的（作者明确要求"新需求改新规矩，不需要兼容"）。
- **`--no-bash` 旧名不再接受**：改名是有意的（外壳早已是 `sh -c`，旧名名不副实）；历史 `CHANGELOG` 里的改名记录属**当时事实**，不改。
- **上传 / 下载 / 改密不经过可配解释器**：有意（ADR-0014 第 6 条）。
- **`-a` 只支持表内解释器**，且正文超约 96KB 会在本地拦下：ADR-0010 的取舍，非缺陷（`open-questions.md` 第一节有"没有第四条通道"的完整论证）。
- **进度条不再有缓动**：有意（去掉了弹簧动画）。拿不准"少个动画是不是退步"时，看 `7a73836` 的提交说明与 issue 01：缓动在一个比帧还密的快照流里本来就看不出来，而它带来的冻结是实测过的真问题。
- **旧 `keywords_error.conf` 下「建连被取消」不带提示**：用户若不换新配置，这个分类名仍会产出、仍聚合成一行，只是没有 tip（会按兜底分类的样式显示）。降级可接受，不是缺陷。
- **沙盒按规范已删**：真机验证用完的 `build/atest*` 不在工作区里。

---

## 去哪看细节

- 决策与取舍：`docs/adr/0014-configurable-interpreters.md`、`docs/adr/0010-deliver-command-body-in-argv.md`（补充节）。
- 解释器那批的三个口子：`docs/issues/configurable-interpreters/open-questions.md`。
- 进度条滞后的根因、证据与修法：`docs/issues/progress-update-lag/issues/01-progress-bar-lags-behind-detail-lines.md`。
- `$0` 那个原始问题的定性与修复：`docs/issues/answer-mismatch-fill/issues/01-unmatched-answer-still-sent.md`。
- 会话交接稿：`docs/issues/configurable-interpreters/handoff-2026-10-07.md`。

## Comments

（暂无）
