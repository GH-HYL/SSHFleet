---
name: audit-conversation-for-WorkBuddy
description: 【对话忠实度审计 · WorkBuddy 版】文档写的，是不是对话最后真正定下的那个。长对话末尾写总结时，模型常凭思维惯性写出已推翻或从没确认过的结论，而这种错在文档内部读起来完全自洽。本技能读 WorkBuddy 会话记录（~/.workbuddy/projects 下的明文 jsonl）逐条比对，只审「文档 vs 对话」，不审代码、不跑实验。单 agent 完成。核对文档是否靠记忆惯性写的时用。
disable-model-invocation: true
---

# 对话忠实度审计（WorkBuddy 版）

审一份**总结性文档**是否忠实于**本轮对话真正得出的结论**。

**只审一件事**：文档写的，是不是对话最后真正收敛的那个。**不审代码、不跑实验、不核实技术事实。**

**由一个 agent 全程完成，不拆并行。** 工作内容单一（读对话、对文档），拆分只会让各路了半边对话、谁也判不出「结论变轨点」。

> 本文与 `audit-conversation-for-DSH` 是同一套审计方法的两套实现，差别只在**怎么翻会话记录**。DSH 的会话是 zstd 多帧压缩的目录树，WorkBuddy 是明文 jsonl，字段名与判据都不同，**别把两者的取数方式混用**。

## 为什么只审对话

长对话的总结文档最容易出的错，不是「事实查错了」，而是**注意力涣散后的思维惯性**：会话经过多轮推翻，最终结论可能第 20 轮才收敛；到第 30 轮写文档时，模型写出**中间某个已废的版本**，或把**自己推的结论**写成「用户确认的」。这类错误**在文档内部读起来完全自洽**——它只会与对话不符。

## 第零步：先定受众，再定审计方向

**同一份文档，受众不同，「忠实」的含义不同：**

| 受众 | 典型文档 | 忠实于什么 | 常见的失真形态 |
| --- | --- | --- | --- |
| **执行者（AI / 后来的维护者）** | ADR、议题、spec、工单 | 用户定的**约束与取舍** | 把助手推导写成用户决定；用户明令的禁令漏掉 |
| **工具使用者** | 手册、帮助、常见问题 | 用户描述的**真实使用场景** | 把手里的推理过程、取证细节写进去，使用者看不懂也不必懂 |
| **使用者可见的文本** | 报错、提示、帮助文本 | 用户**当时**要表达的意图 | 写成只有维护者能看懂的机器话；丢掉了「怎么改」 |

**动手前写一句**：「本次审计对象的受众是 ___，因此重点核对 ___。」**受众不明先问。**

受众还决定**「哪些内容不该出现」**：使用者文档里出现「实验形态」「源码推断」这类内容，**本身就是一条审计发现**——不是内容对不对的问题，是它不该在这里。

## 一、定位会话记录

### 存储布局

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

### 与 DSH 的差异（别照搬那边的做法）

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

## 三、审计四问

### Q1 结论是最后收敛的那个吗

先找**变轨点**：顺 `role: "assistant"` 的 message 搜「撤回」「纠正」「推翻」「改为」，标出轮次；再看文档落在哪一段。长对话会反复推翻，**取最后一次**。

### Q2 每条结论都有用户的明确同意吗

把「决定」类章节逐条拆开标：

| 标记 | 含义 |
| --- | --- |
| ✅ 用户明示 | 原话直接说了 |
| ⚠️ 概括授权 | 「你看着办」「以你判断为准」——可接受，但要标出来 |
| ❌ 助手推导 | 用户从未表态。**必须标出**，不能写成「用户决定」 |

**用户说了一句「你综合判断」，不等于他同意了后来推出的每一条。**

### Q3 有没有把推测写成事实

搜这些句式，逐条 grep 回原话：「他确认」「他提到」「用户明确表示」「实测证明」「已验证」，以及任何暗示是用户原话的引号表述。

**搜不到，就是推测写成了事实。** 最隐蔽的一类：用户描述了**现象**，文档写成了**范围界定**（「只在 X 场景」）。

### Q4 用户说过的话漏了没有

反向查。挑原话里**带约束性**的句子（「别」「不要」「必须」「宁可」），看文档有没有反映。最易漏三类：

- **优先级表态**（「二选一我选 A」）——尤其是与结论方向相反的那半句
- **禁令**（「别干这种活」「别去检测环境」）——注意有**强弱两档**，文档常常只留了弱的那档
- **代价立场**（「代价无所谓」）——文档若用「工作量大」「维护麻烦」否掉候选，就与它相悖

**若文档是面向使用者的（手册、报错、提示），多问一层**：用户描述的是**他遇到什么、想做什么**，文档里有没有变成**机器视角的说明**？真实形态：用户说「执行 `ls` 时中文文件名显示成方块」，报错或文档写成「UTF-8 字节被按 936 码页解释」——**后者是对的，但不是给使用者看的**。判据：把这句话给使用者，他知不知道下一步做什么。

## 四、症状表

| 症状 | 表现 |
| --- | --- |
| 授权漂移 | 写「决策人：用户」，但那条是助手推的。拆成两栏后常常数量对不上 |
| 幽灵确认 | 写「用户确认 X」，对话里搜不到 |
| 选择性引用 | 对结论有利的那句引了，相反的那句没引 |
| 结论停在旧版本 | 写了第 15 轮的结论，第 26 轮已重写 |
| 立场倒置 | 用户说代价无所谓，文档用成本否掉候选 |
| 约束失忆 | 明令的禁令，文档一个字没提 |
| 复审节错载体 | 塞了触发条件 / 补测计划 / 待办清单——那是工单形态 |
| 系统噪音当原话 | 把 `<teammate-message>`、`<task-notification>` 之类当成用户表态 |

## 五、产出

写报告，**不改文档**。

```markdown
# <文档> 对话忠实度审计
受众：<第零步的结论>
## 一句话总评
忠实 / 基本忠实但有偏差 / 有严重偏差
## 逐条核对
| 文档结论 | 依据（U几 / 原话片段） | 判定 |
## 必须修正
## 建议修正
## 无法核实
```

判定只用四个符号：`✅` 有依据 / `⚠️` 无明确授权 / `❌` 与对话矛盾 / `➕` 对话有但文档漏。

**每条必须修正写成三段**：错在哪 → 依据是哪句 → **建议改成什么样**。只有前两段的是清单不是报告。

**区分改结论与改口径**：结论与对话矛盾才动结论；结论对、授权写错，只改变述方式（拆决策人栏，不推翻决策）。

**区分必须与建议**：必须 = 与用户原话直接矛盾、推测当事实、引用错误；其余进建议。**找不出问题就明说**，加一条假问题整份报告就废了。

## 六、如果仓库里有成文的文档纪律

形体层面的问题（复审节写了待办、决策没标授权），**先查仓库有没有把这类要求写下来**：

- 常见落点：`docs/agents/doc-guide.md`——它对 ADR、spec、工单、手册、使用者可见文本**各有一节纪律**。按审计对象的那一份去对。
- **有就引用它的原文**，比自己拟判据站得住，也不与既有约定打架；**没有就明说**，按「与同类文件体例一致」处理。
- 这是**判定依据**，不是审计对象。文档之间的自洽走 `/audit-docs`。

## 边界

- 不审代码、不跑实验、不核实技术事实。
- 不改任何文件。
- **只审主会话**。`<sessionId>/subagents/` 下的内容算助手推导的一部分，不作为依据，也不单独审。
- 会话进行中时，当前步未必落盘；以已落盘为准，并在报告注明截止到哪条用户消息。
- **不跨产品取数**。DSH 的会话在 `~/.dsh/sessions`，格式是 zstd 多帧，判据也不同，那走 `audit-conversation-for-DSH`，别在这里混读。
