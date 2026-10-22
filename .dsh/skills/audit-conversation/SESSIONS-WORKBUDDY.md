# 取数：WorkBuddy 会话记录

本附件只讲**怎么找到会话、哪些行算用户原话**。判据与产出回 `SKILL.md`。

## 一、会话在哪

会话根：`~/.workbuddy/projects/<项目slug>/`

- **项目 slug**：工作区绝对路径，盘符转小写，`\` 与 `/` 都换成 `-`，去掉开头多余的 `-`；中文原样保留，不转义。
  - `D:\Desktop\Code\SSHFleet` → `d-Desktop-Code-SSHFleet`
- **主会话**：`<sessionId>.jsonl`，明文，可直接 `Read`。
- **子 agent**：`<sessionId>/subagents/agent-<taskId>.jsonl`，不算主会话。
- **不用看的**：`<sessionId>.file-rollback.ndjson`（文件回滚记录）、`tool-results/`（超长输出落盘）。

目录不在就模糊匹配：

```bash
ls -1 ~/.workbuddy/projects | grep -i SSHFleet
```

**一个项目目录下常躺着多个 `.jsonl`**，按用户给的时间点或标题定位，**别默认取最新**；标题重名是常有的事。标题在 `type=="ai-title"` 的 `aiTitle` 里，`timestamp` 是**毫秒**。

```bash
# 一眼看有哪些会话
grep -h -o '"aiTitle":"[^"]*"' ~/.workbuddy/projects/*/*.jsonl
```

## 二、哪些行算用户原话

**`type=="message"` 且 `role=="user"`，并且正文包在 `<user_query>...</user_query>` 里。** 其余挂在 `role: "user"` 下的都不算：`<system-reminder>`、`<current_time>`、`<memory_and_skills_reminder>`、`<craft_mode>`、`<user_references>` 这些是注入块。

**最容易误判的两类**：`<teammate-message>`（子 agent 回报）与 `<task-notification>`（后台任务通知）。它们同样以 `role: "user"` 落盘，看起来像用户在说话。

**兜底**：某条消息没有 `<user_query>` 时（早期会话、纯图片、直接粘大段文本），退化为「剥掉 `<system-reminder>…</system-reminder>` 之后的正文」，报告里注明这条属于推断原文，不作为强依据。

## 三、把原话导出来

```bash
python - "$SESSION_JSONL" <<'PY'
import json, re, sys
n = 0
for ln in open(sys.argv[1], encoding="utf-8", errors="replace"):
    try:
        m = json.loads(ln)
    except Exception:
        continue
    if m.get("type") != "message" or m.get("role") != "user":
        continue
    c = m.get("content")
    txt = "".join(x.get("text", "") for x in c if isinstance(x, dict)) if isinstance(c, list) else str(c or "")
    for q in re.findall(r"<user_query>(.*?)</user_query>", txt, re.S):
        n += 1
        print("### U%d (ts=%s) ###\n%s\n" % (n, m.get("timestamp"), q.strip()[:1500]))
PY
```

**把 U1…Un 完整读一遍再动笔**，并当作引用编号。

**三个坑**：文件往往不小，**分段读**，别一次塞进上下文；会话进行中，当前这一步还没落盘，以已落盘为准；`subagents/` 下的不属于主会话。
