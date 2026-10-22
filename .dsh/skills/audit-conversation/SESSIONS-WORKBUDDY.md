# 取数：WorkBuddy 会话记录

本附件只讲**怎么把 WorkBuddy 的会话记录找出来、怎么判断哪些行是用户原话**。判据与产出回 `SKILL.md`。

## 一、存储布局

```
~/.workbuddy/projects/<项目slug>/
  ├─ <sessionId>.jsonl                    ← 主会话，明文 jsonl，直接读
  ├─ <sessionId>.file-rollback.ndjson     ← 文件改动的回滚记录，审计不用
  └─ <sessionId>/
      ├─ subagents/agent-<taskId>.jsonl   ← 子 agent 自己的会话
      └─ tool-results/                    ← 工具超长输出的落盘
```

- **会话根**：`~/.workbuddy/projects`
- **项目 slug**：工作区绝对路径**去掉开头 `/`**，再把 `/` 换成 `-`。中文原样保留，不转义、不编码。
  - `/home/hyl/桌面/Code/SSHFleet` → `home-hyl-桌面-Code-SSHFleet`
  - `/home/hyl/WorkBuddy/2026-10-22-10-52-15` → `home-hyl-WorkBuddy-2026-10-22-10-52-15`
- **主会话**：`<sessionId>.jsonl`，一个文件一个会话，**无压缩，可直接 `Read`**。
- **子 agent**：不跟主会话并列，在 `<sessionId>/subagents/` 下面，文件名是 `agent-<taskId>.jsonl`。

目录不存在就模糊匹配：

```bash
ls -1 "$HOME/.workbuddy/projects" | grep -i "SSHFleet"
```

### 与 DSH 的差异（别照搬另一份附件）

| 维度 | DSH | WorkBuddy |
| --- | --- | --- |
| 编码 | `session.v4.jsonl.zstd`，多帧 zstd，必须按 magic 切帧 | 明文 `.jsonl`，`Read` 或 `json.loads` 逐行即可 |
| 主会话落点 | `<会话目录>/session.v4.jsonl.zstd`，靠 `delegationDepth===0` 认 | `<项目slug>/<sessionId>.jsonl`，层级天然区分 |
| 子 agent 落点 | 裸 `<uuid>/` 平级目录，depth≥1 | `<sessionId>/subagents/agent-<taskId>.jsonl` |
| 用户原话判据 | `type==="user/message"` 且 `source.kind==="user"` | `type==="message"`、`role==="user"`，且内容包在 `<user_query>` 里 |
| 标题来源 | `session/title` 事件 | `type==="ai-title"` 的 `aiTitle` 字段 |
| 时间单位 | — | `timestamp` 为**毫秒** |

### 按标题找会话

```bash
python3 - <<'PY'
import json, os, glob, datetime
root = os.path.expanduser("~/.workbuddy/projects")
rows = []
for f in glob.glob(os.path.join(root, "*", "*.jsonl")):
    if "/subagents/" in f:
        continue
    title, cwd, ts, uq = "", "", None, 0
    with open(f, encoding="utf-8", errors="replace") as fh:
        for ln in fh:
            try:
                m = json.loads(ln)
            except Exception:
                continue
            if ts is None and m.get("timestamp"):
                ts = m["timestamp"]
            if not cwd and m.get("cwd"):
                cwd = m["cwd"]
            t = m.get("type")
            if t == "ai-title":
                title = m.get("aiTitle") or ""
            elif t == "message" and m.get("role") == "user":
                c = m.get("content")
                txt = "".join(x.get("text", "") for x in c if isinstance(x, dict)) if isinstance(c, list) else str(c or "")
                if "<user_query>" in txt:
                    uq += 1
    rows.append((ts or 0, os.path.basename(f)[:-6], cwd, title, uq))

for ts, sid, cwd, title, uq in sorted(rows):
    d = datetime.datetime.fromtimestamp(ts / 1000).strftime("%Y-%m-%d %H:%M") if ts else "????"
    print("%s | %s | %3d 轮用户输入 | %s | %s" % (d, sid[:8], uq, title or "(无标题)", cwd))
PY
```

**同一工作区会有多个会话**（每次新开一个就多一个目录、多一个 jsonl），按用户给的时间点或标题定位，**别默认取最新**。实测本机 21 个项目目录下共有 26 个会话，重名情况真实存在。

### 导出用户原话

```bash
python3 - "$SESSION_JSONL" <<'PY'
import json, re, sys
f = sys.argv[1]
n = 0
for ln in open(f, encoding="utf-8", errors="replace"):
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

**三个坑**：文件常有几 MB（本次会话记录 1.5 MB），**分段读，别一次塞进上下文**——那正好复制了本技能要防的毛病；会话进行中，**当前这一步还没落盘**，以已落盘为准；`subagents/` 下的文件不属于主会话，别混进来。

## 二、只有 `<user_query>` 里的内容是权威

**WorkBuddy 把每轮真实输入包在 `<user_query>...</user_query>` 里，前面塞一堆注入块，全部挂在 `role: "user"` 下。**

实测本会话 24 条 user message 的标签频次：

| 标签 | 次数 | 是什么 |
| --- | --- | --- |
| `<system-reminder>` | 38 | 身份、时间、memory、沙箱提醒，系统注入 |
| `<user_query>` | 19 | **唯一真实输入** |
| `<current_time>` | 19 | 时间戳注入 |
| `<memory_and_skills_reminder>` | 19 | 系统提醒 |
| `<craft_mode>` | 14 | 模式切换通知 |
| `<teammate-message>` | 4 | 子 agent 回报 / 团队调度 |
| `<task-notification>` | 1 | 后台任务完成 |
| `<user_references>`、`<attached_files>` | 2 | 附件与文件引用 |

**最容易误判的一类**：`<teammate-message>` 和 `<task-notification>`。它们同样以 `role: "user"` 落盘，看起来像用户在说话，但那是框架自动通知。子 agent 自己的会话（`subagents/agent-*.jsonl`）第一条 user 消息就是 `<teammate-message teammate_id="team-lead">`——那是助手派出去的活，**一律不算用户原话**。

**兜底**：某条消息若没有 `<user_query>`（早期会话、纯图片输入、直接粘贴大段文本），退化为「剥掉所有 `<system-reminder>…</system-reminder>` 块后剩余的正文」，并在报告里注明该条属于推断原文，不作为强依据。

```bash
# 快速核对某会话里 user_query 与注入块的比例
python3 - "$SESSION_JSONL" <<'PY'
import json, re, sys, collections
tag = collections.Counter()
for ln in open(sys.argv[1], encoding="utf-8", errors="replace"):
    try: m = json.loads(ln)
    except Exception: continue
    if m.get("type") != "message" or m.get("role") != "user": continue
    c = m.get("content")
    txt = "".join(x.get("text","") for x in c if isinstance(x,dict)) if isinstance(c,list) else str(c or "")
    for t in re.findall(r"<([a-z_-]+)[ >]", txt): tag[t] += 1
for k, v in tag.most_common(): print("%-30s %d" % (k, v))
PY
```

**把 U1…Un 完整读一遍再动笔**，并当作引用编号。几十轮的长对话，真·原话常常只有二三十条。
