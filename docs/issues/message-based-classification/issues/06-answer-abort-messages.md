# 06: 代填与中断两摊改用报文

**要做的**：代填没送出去、代填用尽后超时、任务被取消、建连被取消这四种情况由配置认领。中断两摊的报文已在 02 落定（它们是 11 个常量里的一部分），本工单补 `answer` 一摊，并把**中止词命中之后不再写死分类名**这件事落定。

**实施入口**：`docs/issues/message-based-classification/README.md`。

**被谁阻塞**：05（改密一摊已迁完）。

Status: done（2026-10-20）

## 验收

- [ ] `answer` 一摊：两件事改为发报文并由判据文件认领：
  - `sshfleet.answer.missed: <n> answers not delivered`
  - `sshfleet.answer.exhausted_timeout: all answers sent but no output before timeout`
- [ ] `answer` 一摊的两条由代填收尾处的导出纯函数产出（入参是未送出的条数、是否用尽、是否超时），与 02 立下的做法一致。
- [ ] `abort` 一摊：确认 02 落下的 `sshfleet.abort.canceled` 与 `sshfleet.abort.connect_canceled` 两处在两种判定下都成立——**已连上被中断**（`abort.canceled`）与**还没连上就被中断**（`abort.connect_canceled`）分别对应今天的「任务已取消」「建连被取消」。
- [ ] **中止词命中后不再由代码写死分类名**：删掉 `verdict.Judge` 里 `if res.AbortLine != "" { 分类 = "密码过期" }` 那条分支。`AbortLine` 仍然作为事实保留（呈现层据此说明会话为何提前结束），其原文**并入待匹配文本**，分类由第一组判据表给出。
- [ ] 用例证明中止路径仍然给出「密码过期」：以 `AbortLine`（例如 `WARNING: Your password has expired.`）为输入，判据表第一组把它认成「密码过期」。
- [ ] **代填的中止词在交互链路上按远端提示命中**（按分类名「密码过期」取关键词）。这条链路**代码里已生效**（调度侧取出后注入），本工单做端到端复核：把配置里「密码过期」那条的 `keywords` 换掉，中止行为随之变化；把它那条整组删掉，中止行为消失（不再有中止词）；改它的 `name` 后启动期的守卫应报错亮明，而不是静默干等到超时。
- [ ] 分类语料全绿。

## Comments

- 2026-10-20：原验收写着「中止词在交互链路上按报文命中」——**这是错的**，已改。中止词看的是**远端说什么**（服务端提示），不是工具报文；它是配置里那条分类的关键词。
- 2026-10-20：中止词的实质（作者核对）：命令已经跑起来了，远端把会话切进改密向导，工具从输出里认出那几行提示，于是**主动中止会话**——继续喂下去毫无意义。所以「中止」是工具的动作，「密码过期」是远端的事实，分类依据是后者。
- 2026-10-20：**代码侧已核实全部就位**：`AnswerMessage` 与 `AbortMessage` 两条纯函数由 02 落地；`verdict.Judge` 里没有 `if res.AbortLine != "" → 密码过期` 那条分支，`AbortLine` 只作为事实并入待匹配文本（`verdict.go:109`）。
- 2026-10-20：**真机复核（代填四档）**：
  - 正常代填（两条触发词都命中）→ 执行成功；
  - 触发词写错 → 错误详情 `sshfleet.answer.missed: 1 answer not delivered` + 未送出代填那行；
  - 表已送空仍超时 → `sshfleet.answer.exhausted_timeout: all answers sent but no output before timeout`；
  - 中止词命中（远端输出 `WARNING: Your password has expired.`）→ **0.013 秒收场**，分类「密码过期」，证明中止链路按远端提示命中。
- 2026-10-20：**换关键词验证中止行为随之消失**：把「密码过期」的 keywords 换成远端不会说的串 → 不再中止，会话干等到超时，分类落「执行超时」。**关键细节**：必须**两处都换**（第一组与第二组各有一条同名分类，`PasswordExpired()` 取两块并集），只换第一组不生效——这一点与 05 的改名发现同源。
- 2026-10-20：**修了一处顺序问题**：`触发词未命中` 原先排在 `执行超时` 之后，而代填没送出去时那一格里必然同时有 `run.timeout`，于是整类永远见不到，实测落成了「执行超时」。已把「执行超时」挪到两条代填分类之后。现在同样的场景落「触发词未命中」。
- 2026-10-20：**中断两档的真机复核**（`internal/ssh/abort_messages_test.go`，`-tags integration`）：连上之后取消 → `sshfleet.abort.canceled`；还没连上就取消 → `sshfleet.abort.connect_canceled`。为什么不真按 Ctrl+C：信号在 Windows 上送达 MSYS 进程不可靠，而这里要验的是 context 取消后结果里留下的事实，那正是 Ctrl+C 在程序内部的形态。
- 2026-10-20：**测试环境备注**：代填用的交互脚本放在目标机 `/tmp`（`-s` 指本机脚本，`-c "bash /tmp/xxx.sh"` 才是下发）；`-a` 的 CSV 每行是「代填内容,触发词」，写反了不会报错、只会认不出触发词。用完已删。
