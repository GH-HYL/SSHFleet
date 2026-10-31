# WorkBuddy 会话记录

WorkBuddy 的会话记录存在本机，需要时直接读。

**位置**：`~/.workbuddy/projects/<项目slug>/`

- **项目 slug**：工作区绝对路径，`/`、`\`、`:` 一律换成 `-`，开头多余的 `-` 去掉；Windows 盘符转小写。`D:\Desktop\Code\SSHFleet` → `d-Desktop-Code-SSHFleet`
- **主会话**：`<sessionId>.jsonl`，明文 JSONL。
- **不看的**：`subagents/`（子 agent）、`tool-results/`（超长输出落盘）、`*.file-rollback.ndjson`（回滚记录）。

目录找不到就模糊匹配：

```bash
ls -1 ~/.workbuddy/projects | grep -i 关键字
```

**找哪一条**：多半只给会话标题，也可能什么线索都没有；别默认只查当前工作区那个目录。

- **给了标题**：标题是明文，跨工作区直接 grep：
  ```bash
  grep -li "标题里的几个字" ~/.workbuddy/projects/*/*.jsonl
  ```
  命中多条再按 `timestamp`（毫秒）挑，别默认取最新；标题可能缺失、可能重名。
- **没给线索**：同一工作区里按 `timestamp` 取最新。

文件可能几 MB，别整份 `Read`。

**读的时候**：用户消息是 `type=="message"` 且 `role=="user"`，正文在 `content[]` 的 `text` 里（块类型为 `input_text`）。`role:"user"` 下不全是用户原话，`<system-reminder>`、`<teammate-message>`（子 agent 回报）、`<task-notification>`（后台任务通知）这类注入块也挂在那儿；用户本人说的是 `<user_query>` 里的内容。

**弹窗问答不在 message 里**：`AskUserQuestion` 的回答落在 `type=="function_call_result"`、`name=="AskUserQuestion"` 的 `output.text`，形如「 · 问题 → 答案」。点选项和手打都落这一处，text 就是选项名或打的字。只抓 `<user_query>` 会整条漏掉。
