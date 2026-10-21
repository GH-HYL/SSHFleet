# 03: 会话与执行两摊改用报文

**要做的**：连上一台机器但会话建不起来、或申请终端失败时，错误行同样走报文体系；命令执行超时这类由执行侧编出的提示也有了统一的报文形态。分类归属与改造前一致。

结构性分类的报文已在 02 落定（`sshfleet.abort.canceled`、`sshfleet.session.exit_missing` 等），本工单只补 `session` 与 `run` 两摊的**错误文本**。

**实施入口**：`docs/issues/2026-10-20-1604-报文式失败分类/README.md`。

**被谁阻塞**：02（报文格式、校验规则终态、结构事实的发报点均已就位，照抄即可）。

Status: done

- [ ] `session` 一摊：创建会话失败、申请终端失败、创建输入通道失败三处中文前缀改为报文，同一情形只用一个环节名（`create_failed` / `pty_failed` / `channel_failed`），三条通道（命令 / 代填 / 改密）共用。
- [ ] `run` 一摊：`命令执行超时(5s)` 改为报文。**两处各编一次的那句必须收成一条**（命令通道与代填通道各有一处），同一情形只允许一个环节名。
- [ ] 判据文件对应条目关键词按报文改写；`无法建立会话`、`终端分配失败`、`执行超时`、`握手超时`、`会话被中断` 等分类归属不变（语料全绿）。
- [ ] 呈现层不动：报文原文直接上屏，没有译文层要补。
- [ ] 相关测试期望值同步更新；以中文错误文本作输入的用例改为以报文作输入，期望分类不变。

## Comments

- 2026-10-20：「命令超时两处各编一次」的收口方式：两条路径最终都经过 `endFactsOf`（命令 / 代填 / 改密三条通道的唯一收尾映射点），报文前缀在那里统一加，两个调用点不再各自拼。

- 2026-10-20：**已实施**（提交 `496878c`）。02 落地时三处会话前缀与两处超时已经改完，03 只剩两件事：把「命令被拒」从库的措辞里摘出来、跑真机用例。验收逐条：
  - `session` 一摊：`create_failed` / `pty_failed` / `channel_failed` 三个环节名在命令、代填、改密三条通道上共用，同一情形只产出一句报文。
  - `run` 一摊：两处超时（`exec.go` 的 `runWithTimeoutAndCancel` 与 `interactive.go` 的 `driveSession`）收成同一个 `run.timeout`，措辞统一为 `exceeded <duration>`。
  - 新增 `run.rejected`：`NewSession` 返回的 `*ssh.OpenChannelError` 由 `endFactsOf` 翻成报文，判据表「命令被拒绝」改挂它、旧措辞保留兜底。
  - **一处实测出来的边界**：命令通道上 `NewSession` 先失败，已经被 `session.create_failed` 接住，所以 `run.rejected` 实际只在**代填通道的 `Start` 被拒**时才走到。这不是没生效，是两条通道的分工不同——记在这里，免得后来的人按「命令被拒」去找它却找不到。
  - 真机验证（WSL Ubuntu-26.04）：命令成功 → 执行成功；`sleep 30` 配 `-t 2` → `sshfleet.run.timeout: exceeded 2s` / 执行超时；nologin 账号 → 第二组按远端原话认出「登录被禁止」，错误详情里原文完整保留。
  - 语料补 5 条（`channel_failed`、`pty_failed`、`run.rejected`、`session.exit_missing`、`run.timeout`），配置一致性断言补 `run.rejected` 与 `channel_failed`。
  - `go build ./...`、`go vet ./...`、`go test ./...` 全过。
