# 05: 改密一摊改用报文，「密码过期」与中文名脱钩

**要做的**：改密卡住时的四种形态（提示词没认出 / 没等到提示 / 远端无响应 / 改密未完成）与「密码未过期」全部由配置认领；同时把「密码过期」从**代码产出的分类名**上摘下来——它是远端说的话，不是工具的报文（ADR-0021 决定第 9 条）。

**实施入口**：`docs/issues/message-based-classification/README.md`。

**被谁阻塞**：04（配置认领这条路已在传输一摊验证过）。

Status: ready-for-agent

## 验收

- [ ] `passwd` 一摊：改密卡住四种形态、密码未过期改为发报文，由改密配置认领：
  - `sshfleet.passwd.no_signal: no password prompt seen before timeout`
  - `sshfleet.passwd.no_reply: password fed but no reply before timeout`
  - `sshfleet.passwd.no_prompt: passwd exited without asking for input`
  - `sshfleet.passwd.give_up: no further prompts after answers used up`
  - `sshfleet.passwd.not_expired: no password-expiry prompt seen`
- [ ] 这五条由 `passwdsession.go` 收尾处的**导出纯函数**产出，入参是 `TimedOut` / `PasswdFedAny` / `PasswdEarlyClose` / `EnterSignalSeen` 与「远端有没有说过话」（今天那两条分支靠 `Error == ""` 判断的就是这件事）。**远端说了话就别再补报文**——那种形态让判据表按远端原话认，与今天的让位口径一致。
- [ ] **「密码过期」不发报文**：它是远端说的话（改密向导那几行），工具只是把输出读了回来。分类常量删除后，它的分类在命令 / 脚本模式下由第二组判据表按输出原文给出。
- [ ] 改密的**入场信号**与 `main.go` 里的守卫：仍取 `Keywords.PasswordExpired()`（按分类名「密码过期」去判据表取关键词）。用例证明：
  - 把那条分类的 `keywords` 换成别的远端提示 → 改密照着新关键词认（这是要保留的能力）；
  - 改那条分类的 `name` → 入场信号取不到关键词，`main.go` 的守卫应当**报错退出**、而不是静默干等到超时。这是既有实现的事实，本工单只把它写进用例，不改行为。
- [ ] 代填的中止词同样取 `Keywords.PasswordExpired()`（同一条分类的关键词）。**它在代码里已经是这个取法、已经生效**（`batch.go` 取出后注入 `InteractiveInput.AbortKeywords`），本工单只确认改造后它没有被连带改坏。**06 会再跑一遍端到端用例**并补记在 06 的 Comments 里。
- [ ] 判据文件与配置文件里关于「由程序定性 / `code_defined`」的头注释改写为报文口径（02 已删字段，这里清掉文档层的说法）。
- [ ] 分类语料与改密相关测试全绿。

## Comments

- 2026-10-20：原验收写着「入场信号改为按报文取向」——**这是错的**，已改。入场信号来自**远端**（`WARNING: Your password has expired` 这类服务端提示），不是工具报文。
- 2026-10-20：补记实际链路（作者核对后确认）：密码过期时**连接与认证全部成功**，看不到任何异常；真正的事发生在会话内部——请求执行命令时 PAM 先把会话切成改密对话，标准输出收回来的是改密向导的话，会话随即以**非 0** 收场。那个非 0 退出码来自 PAM 改密对话、不是命令的，但**分辨它属于谁既做不到也没必要**：它就是「会话没正常结束」的信号，把判定导向第二组；真正定分类的是输出里的原话。
- 2026-10-20：「分类名本身不能改」这句要收回，同时反过来纠正一处：入场信号与中止词是**按名查表**（`Keywords.PasswordExpired` 找的是名为「密码过期」那条），所以改它的 `name` **会**让查表落空——先前写的「改名不影响」是错的。改 `keywords` 才是真正生效的调整口。本工单要摘掉的是「名字由代码常量产出」这件事，不涉及改名能力。
