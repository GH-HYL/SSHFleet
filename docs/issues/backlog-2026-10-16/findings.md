# 存量问题清单（2026-10-16）

**来源**：2026-10-16 核对最新一批 ADR / spec / issues 的实际状态时，把「记录与事实不符」与「遗留未收口」的条目集中登记，供作者择期集中整改。
**状态**：**全部未处理**。
**性质**：纯记录，不含修复方案——每条只写「现象 + 证据 + 影响」，怎么处置等开轮时再定。

> 收进来的标准：只收**已核实属实**的（有 file:line 或可复现证据）。未经核实的猜测不收。

---

## 一、记录失真：`Status:` 不代表现状（本仓普遍现象）

### 现象

`docs/issues/<slug>/` 下各文档的 `Status:` 行是**写下当时**的状态，后续轮次多数没有再维护。
接手的人会把它当成现状，据此判断「这活还能不能干、要不要干」。

**同一件事的状态在本仓可能记在三处，三者可以互相矛盾**：

```text
① 文件名后缀      spec-已闭环.md / issues/04-closeout-and-verify-已闭环.md
② 文件内 Status 行
③ 工单自己的 Status 行（与 spec 的 Status 各写各的）
```

### 已核实的失真点

| 位置 | 写的 | 实际 |
| --- | --- | --- |
| `docs/issues/usage-error-message-audit/spec.md:3` | `Status: ready-for-agent` | 三条工单（`issues/01`–`03`）**全部 resolved**，内容已随 `[9.1.5]` 发布 |
| `docs/issues/configurable-interpreters/handoff-2026-10-07.md:3` | `Status: ready-for-agent` | 该稿首节写「已完成、已提交」，待办只剩「闭环历史遗留工单」——**残留的 ready-for-agent 会让人以为功能本身没做完** |
| `docs/issues/configurable-interpreters/audit-handoff.md:3` | `Status: ready-for-human` | 同上，与功能已完成的事实不符 |
| `docs/issues/backlog-cleanup/handoff-2026-10-08.md:3` | `Status: ready-for-agent` | 该稿是「清理历史 ADR/issues」的任务书，**任务本身未做**——状态恰好没错，但它没说这件事还在不在 |

**该文件自己就提醒过**：`docs/issues/backlog-cleanup/handoff-2026-10-08.md` 开头写着
「文档里写的 `Status` 是**当时**写下的，不代表现在，别拿它直接下结论」。

### 影响

- 接手的人**不能信 `Status`**，只能回代码与提交里逐条核；核完的结论没有回写，下一轮再核一遍。
- 「集中整改」的排序会被带偏：已完成的被当成待办，真待办的被当成已完成。

### 待作者定

1. 议题级的「单一状态源」放哪——spec 的 `Status`？文件名后缀？还是另立一张总表。
2. 本轮要不要把上面已核出的失真点先订正。

---

## 二、`security-audit` 有报告、缺规格与工单

**位置**：`docs/issues/security-audit/`

**现象**：该议题有完整三份报告（`final-report.md` 任务书 + `live-report.md` 实测 + `static-report.md` 静态分析）与 `HANDOFF.md`，
但**没有 `spec.md`、没有 `issues/` 目录**——按本仓约定（`docs/agents/issue-tracker.md`），它还不具备「可实施」的形态。

**已核实：三条缺陷都还在**（2026-10-16 只读核对）

```text
S1/S2  非 shell 脚本的危险内容既不检测（漏报），又误报阻断整批
       证据：modules/SSHFleet_Go/internal/dangercheck/check.go:4 仍是 mvdan.cc/sh/v3/syntax
             （用 shell 语法解析一切内容），而解释器表支持 15 种
S3     --yes 下放行危险命令，终端无提示
       证据：modules/SSHFleet_Go/main.go:312 的 autoNote 只经 Confirm.Trace 进日志；
             main.go:307–336 非交互分支不上屏
```

**未拍板**：S1/S2 的处置方向（`final-report.md` 第四节给了三条路），作者尚未选定。

**影响**：**本清单里唯一的高危项**。S2（误报）会让整批 2000 台一个节点都跑不了，且 `forbidden` 档没有绕过手段。

---

## 三、`backlog-2026-10-13` 存量未处理

**位置**：`docs/issues/backlog-2026-10-13/findings.md`

该文件自述「**全部未处理**」：P1（`keywords_passwd.conf` 一条关键词永远匹配不上 + 两处文档陈旧）、
P2（`interactor.go` 的 `Prompt` / `PromptPassword` 无调用者）、T1（batch / confirm / log 三包无测试）、R1（待复核）。

内容照旧成立，**本条不重复登记，处置时直接读该文件**。

---

## 四、本轮：`win10-ansi-color` —— 当天已闭环

**位置**：`docs/issues/win10-ansi-color/`

规格 + 工单 + 本地可复现的探针（`build/atest-8/probe7`）+ ADR-0017 齐备，
已于 2026-10-16 当天实施并复查完：`ea5cec1`（实施，含 CHANGELOG `[9.1.6]`）、
`c44048f`（工单与规格回填）、`00e3770`（复查报告）。

**本节不再有待办**，留在这里只说明它已闭环（原先记的「文档尚未进版本库」也已由 `c16afaa` 提交解决）。

---

## 五、本轮收尾时冒出的待处理项（2026-10-16）

### ~~F1 · `docs/条款体检-2026-10-13.md:188` 引用了规范的旧标题~~ —— 已闭环（2026-10-16）

作者当天把 `docs/条款体检-2026-10-13.md` 整个文件删掉了，这处旧标题引用随之消失；
何况规范当天也换成了新版（章节改为「约束 / 流程 / 结构 / 产出」），旧标题本就不存在。

### F2 · `win10-ansi-color/` 下两份文档的 `Status:` 没跟到实际

| 文件 | 现在挂着 | 实际 |
| --- | --- | --- |
| `findings-2026-10-16.md` | `ready-for-human` | 排查已完成，实施也已完成 |
| `handoff.md` | `needs-info` | 历史交接背景，其中若干前提已被 findings 推翻 |

归入第一节「`Status:` 不代表现状」，这里只把具体位置点出来。

### F3 · `build/` 下的沙盒该按天清一遍

`build/` 下有 `atest-1` 至 `atest-8` 八个沙盒。按规范第一节的按天清理规则（当天与昨天留着，
前前天及更早的整个删掉），开工前该扫一眼哪些已过期。本轮复现用的 `atest-8` 是当天建的，不动它。

---

## 附：本机环境（非议题，一并登记）

### E1 · git 报 dubious ownership（2026-10-16 已修）

**现象**：工作区在 Windows 上，`git` 一切命令报
`fatal: detected dubious ownership in repository at 'D:/Desktop/Code/SSHFleet'`。

**根因**：该目录的 NTFS 属主是 `NT AUTHORITY/SYSTEM (S-1-5-18)`，当前用户是
`WIN-HIMKR9SAD8C/Administrator (S-1-5-21-…-500)`。git 2.35.2+ 因 CVE-2022-24765 的防护，
在「仓库属主 ≠ 当前用户」时拒绝操作。**与「工作区从 Linux 复制过来」间接相关**：
Linux 的 uid/gid 属主没法带到 NTFS，复制后目录 Owner 落到了 SYSTEM。

**处置**：已加入全局白名单（git 官方推荐做法）

```bash
git config --global --add safe.directory D:/Desktop/Code/SSHFleet
```

`git status` / `git log` 现已正常。撤销：`git config --global --unset safe.directory D:/Desktop/Code/SSHFleet`。

更彻底的一条路是改目录属主（`takeown` / `icacls`），没必要，未做。

---

## Comments

- 2026-10-16：作者点名登记第一节（`Status:` 失真），并指示把 `win10-ansi-color` 以外的遗留一并记入本文件。
- 2026-10-16（收尾）：第四节改为「当天已闭环」；新增第五节，登记收尾时冒出的三条（规范旧标题引用、
  `win10-ansi-color` 下两份文档的状态、`build/` 沙盒按天清理）。
