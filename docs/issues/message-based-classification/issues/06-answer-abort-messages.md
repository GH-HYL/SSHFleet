# 06: 代填与中断两摊改用报文

**要做的**：代填没送出去、代填用尽后超时、任务被取消、建连被取消这四种情况由配置认领。中断两摊的报文已在 02 落定（它们是 11 个常量里的一部分），本工单补 `answer` 一摊，并把**中止词命中之后不再写死分类名**这件事落定。

**被谁阻塞**：05（改密一摊已迁完）。

**状态**：ready-for-agent

## 验收

- [ ] `answer` 一摊：两件事改为发报文并由判据文件认领：
  - `sshfleet.answer.missed: <n> answers not delivered`
  - `sshfleet.answer.exhausted_timeout: all answers sent but no output before timeout`
- [ ] `abort` 一摊：确认 02 落下的 `sshfleet.abort.canceled` 与 `sshfleet.abort.connect_canceled` 两处在两种判定下都成立——**已连上被中断**（`abort.canceled`）与**还没连上就被中断**（`abort.connect_canceled`）分别对应今天的「任务已取消」「建连被取消」。
- [ ] **中止词命中后不再由代码写死分类名**：删掉 `verdict.Judge` 里 `if res.AbortLine != "" { 分类 = "密码过期" }` 那条分支。`AbortLine` 仍然作为事实保留（呈现层据此说明会话为何提前结束），其原文**并入待匹配文本**，分类由第一组判据表给出。
- [ ] 用例证明中止路径仍然给出「密码过期」：以 `AbortLine`（例如 `WARNING: Your password has expired.`）为输入，判据表第一组把它认成「密码过期」。
- [ ] **代填的中止词在交互链路上按远端提示命中**（取 `Keywords.PasswordExpired()`，05 已改）。用例：把配置里「密码过期」那条的 `keywords` 换掉，中止行为随之变化；把它那条整组删掉，中止行为消失（不再有中止词）。
- [ ] 分类语料全绿。

## Comments

- 2026-10-20 原验收写着「中止词在交互链路上按报文命中」——**这是错的**，已改。中止词看的是**远端说什么**（服务端提示），不是工具报文；它是配置里那条分类的关键词。
- 2026-10-20 中止词的实质（作者核对）：命令已经跑起来了，远端把会话切进改密向导，工具从输出里认出那几行提示，于是**主动中止会话**——继续喂下去毫无意义。所以「中止」是工具的动作，「密码过期」是远端的事实，分类依据是后者。
