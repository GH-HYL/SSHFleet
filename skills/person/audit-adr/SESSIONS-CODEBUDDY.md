# CodeBuddy 会话记录

CodeBuddy 的会话记录存在本机，需要时直接读。

**位置**：`~/.codebuddy/projects/<工作区slug>/`

- **slug**：工作区绝对路径，`/`、`\` 换成 `-`，`:` 直接删掉（所以 `D-` 而不是 `d-` 或 `D--`）；盘符大小写不定，`D-` 和 `d-` 都见过。`D:\Desktop\Code\SSHFleet` → `D-Desktop-Code-SSHFleet`
- **主会话**：`<sessionId>.jsonl`，明文 JSONL。
- **不看的**：同名的 `<sessionId>/` 目录（`subagents/`、`tool-results/`）、`storage.json`。

目录算不出来就先模糊匹配：

```bash
ls -1 ~/.codebuddy/projects | grep -i 关键字
```

**找哪一条**：多半只给会话标题，也可能什么线索都没有；别默认只查当前工作区那个目录。

- **给了标题**：标题是明文，跨工作区按关键字模糊搜，别指望一字不差：
  ```bash
  grep -li "标题里的几个字" ~/.codebuddy/projects/*/*.jsonl
  ```
  命中的文件里看 `type=="ai-title"` 记录的 `aiTitle` 和 `cwd`；不是每个会话都有标题，没有就退回正文找。**`cwd` 只证明在哪个工作区，不证明是哪一条**——同一工作区常有多条，读首条用户消息比对。
- **没给线索**：同一个工作区里按 `timestamp`（毫秒）取最新。

文件可能几 MB，别整份 `Read`。

**读的时候**：用户消息是 `type=="message"` 且 `role=="user"`，正文在 `content[]` 的 `text` 里（块类型 `input_text`），**没有 `<user_query>` 包裹**，就是纯文本。以 `<` 开头的是注入块或斜杠命令，不算——`/xxx` 触发技能时整篇技能正文会当成一条用户消息落盘，几千字，最容易被当成「用户说了什么」。
