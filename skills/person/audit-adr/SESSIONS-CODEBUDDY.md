# 取数：CodeBuddy 会话记录

本附件只讲**怎么找到会话、哪些行算用户原话**。审计方法见同目录 `SKILL.md`。
另两份 `SESSIONS-WORKBUDDY.md` / `SESSIONS-DSH.md` 的格式与这份**都不同**，别混用。

## 一、会话在哪

会话根：`~/.codebuddy/projects/<工作区slug>/`

- **slug**：工作区绝对路径，盘符**保留大写**，去掉 `:`，`\` 与 `/` 都换成 `-`。
  - `D:\Desktop\Code\SSHFleet` → `D-Desktop-Code-SSHFleet`（注意与 WorkBuddy 的小写形式不同）
- **主会话**：`<sessionId>.jsonl`，明文，可直接读。
- **子 agent**：同名的 `<sessionId>/` 目录下，不算主会话。
- **不用看的**：`storage.json`。

目录不在就模糊匹配：

```bash
ls -1 ~/.codebuddy/projects | grep -i SSHFleet
```

**一个项目目录下可能躺着多个 `.jsonl`**，按时间或标题定位，别默认取最新。

## 二、哪些行算用户原话

**`type=="message"` 且 `role=="user"`**，正文在 `content` 数组里，每个元素形如 `{"type":"input_text","text":"…"}`。

**与 WorkBuddy 最大的差别：这里没有 `<user_query>` 包裹。** 正文就是纯文本，原样落盘。

排除两类：

- **注入块**：正文以 `<system-reminder>` 之类开头（`content` 里同样带着）。
- **斜杠命令**：以 `/xxx` 触发技能时，**整篇技能正文会当成一条用户消息落盘**（形如 `<command-message>audit-adr</command-message>` 加技能全文）。它时长几千字，极易被当成「用户说了什么」误引。

## 三、把原话导出来

```bash
python - "$SESSION_JSONL" <<'PY'
import json, sys
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
    s = txt.lstrip()
    if s.startswith("<system-reminder") or s.startswith("<command-message"):
        continue
    n += 1
    print("### U%d (ts=%s) ###\n%s\n" % (n, m.get("timestamp"), txt.strip()[:1500]))
PY
```

**把 U1…Un 完整读一遍再动笔**，并当作引用编号。

## 四、三个坑

- **会话进行中**，当前这一步可能还没落盘；以已落盘为准，缺口在报告里注明。
- **文件不小**（一次长会话 2MB 量级），**分段读**，别一次塞进上下文；导出后先落 `build/` 再读。
- **用户原话常常是语音转的**，有错字（「03 角色工单」其实指「03 号决策工单」、「阶段」其实指「决断」）。引原话时保留原样，理解时按上下文校正。
