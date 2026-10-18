# 文档存量清理 · 规格

Status: done（2026-10-17 本轮完成）

> **本议题做的事**：把历史遗留的 ADR 与 issues 逐个走一遍，能闭环的闭环掉。
>
> **来历**：原开发者 2026-10-08 留下过一份任务书（见「补充说明」）。该任务当时没做，且它写的范围已经过期——本议题以实际核查为准，不沿用旧范围。

---

## 问题陈述

版本迭代多轮之后，`docs/issues/` 下积下大量文档，其中相当一部分记录与事实不符：

- 文件名带状态后缀（`spec-已闭环.md`），而状态本该由 `Status:` 行承载
- `Status:` 值有 `resolved`、`已闭环`、`已实施` 等多种写法，都不在本仓约定的五个合法值里
- 同一件事的状态记在三处（文件名后缀 / spec 的 Status / 工单的 Status），三者可以互相矛盾
- 有自造的文件名（`handoff*`、`findings`、`audit`、`recheck`、`deferred`、`open-questions`、`plan-coverage` 等），载体约定里没有这些位置
- 部分文档引用的提交号在仓储里根本不存在

接手的人不能信 `Status`，只能回代码与提交里逐条核；核完的结论没有回写，下一轮再核一遍。

---

## 方案

按技能载体约定逐个归位，分四类处置：

1. **文件名归位**：`spec-*.md` 一律改回 `spec.md`；工单名去掉状态后缀。
2. **状态归位**：全部状态集中到 `Status:` 行，值统一到合法集合。
3. **附加文档归位**：能并进 `## Comments` 的并入后删除；承载证据底本的保留并改名为约定内的形态；已退役的删除。
4. **证据更正**：查不到的提交号换成现仓储的真号，并注明原号失效的原因。

---

## 使用者故事

1. 作为接手这个仓库的人，我想在 `docs/issues/<slug>/` 下直接看到 `spec.md`，这样我不必猜某个 `spec-已闭环.md` 是不是规格。
2. 作为接手这个仓库的人，我想让每个文件的 `Status:` 说的是真的，这样我不必回代码里重新核一遍。
3. 作为接手这个仓库的人，我想让状态只出现一处，这样我不会遇到「文件名说已完成、Status 说待做」的矛盾。
4. 作为照工单干活的人，我想让工单里引的提交号 `git show` 得出来，这样我能看见当时的改动。
5. 作为维护者，我想知道哪些附加文档是「证据底本」可以留、哪些是过期交接稿该删，这样目录不会越长越乱。

---

## 实现决策

**新增 `done` 状态值**：技能原版五个标签里没有「已完成」，因为它假设工单做完就被「关闭」。本仓用本地 markdown，文件关不掉，因此新增 `done`，并在 `triage-labels.md` 里注明「技能原版无此值、本仓新增」。

**允许存在的证据底本只有三类**（写进 `issue-tracker.md`）：`findings-<日期>.md`（根因分析）、`audit.md`（实测报告，多条工单共用）、`<something>-copy.md`（文案底本，有回写义务）。判据是「这份材料是某次做完了的记录，还是后来人要依据的东西」。

**其余自造文件名一律不认**：交接稿过期即删；验收/复跑记录归 `## Comments`；「明知有代价不修」的裁定归 ADR。

**提交号更正**：2026-09-30 做过一次 `git filter-repo` 全历史重写（407 笔提交、SHA 全变），此前的提交号已不可考。处置是换成现仓储的真号，并注明原号失效原因。

---

## 测试决策

**本议题不产出代码，不设测试接缝。**

核验方式是逐条比对：文件名与目录结构对照载体约定、`Status` 值对照合法集合、提交号逐个 `git cat-file -t` 验存在性。这些是人工核对，不做自动化——一次性清理，建自动化不划算。

---

## 不在本次范围

- 修改任何代码。本议题只做文档层面的核实与归位。
- 议题内容的实质改写（除状态、文件名、附加文档归位外，正文不动）。
- 新发现代码缺陷的修复——那登记在 `docs/issues/defect-scan-2026-10-17/`。

---

## 补充说明

### 实际范围（与 2026-10-08 旧任务书的差异）

旧任务书写的是「`docs/adr/` 14 份、`docs/issues/` 12 个目录 78 份文档」，`git log -1` 为 `7a73836`。核实时实际情况：

| 项 | 旧任务书 | 实际 |
| --- | --- | --- |
| ADR | 14 份（0001–0014） | 清理前 17 份（0001–0017）；本轮新建 0018，现共 18 份（0001–0018） |
| 议题目录 | 12 个 | 清理高峰期 19 个；交付时 17 个 |
| 文档 | 78 份 | 清理前 89 份；清理后 81 份 |

**旧任务书里的「起点线索」也已被推翻**：它说 `mode-centralization` 与 `rewrite-to-go` 的 `Status` 疑似没回填——核实后这两处早该标 done，属状态值写法问题而非「没回填」。

### 旧任务书的两处说法已失实

1. **「`docs/issues/**` 刻意不进版本库、不要 `git add -f`」** —— 已失实。`.gitignore` 第 34–35 行已放行 `docs/issues/` 与 `docs/issues/**`，工单目录随仓库追踪。
2. **「`git log -1` = `7a73836`、版本 9.0.1」** —— 是 2026-10-08 的快照，不代表现在。

### 本轮做了什么

见 `## Comments`。

### 附：git 报 dubious ownership 的处置（2026-10-16，留档）

> **为什么留这里**：这是本机环境的排障记录，原在 `backlog-2026-10-16/findings.md`。该清单撤销时曾一并删除，经审计指出——它是**唯一**一份记录，换机器再撞上还得从头查一遍。

**现象**：工作区在 Windows 上，`git` 一切命令报
`fatal: detected dubious ownership in repository at 'D:/Desktop/Code/SSHFleet'`。

**根因**：该目录的 NTFS 属主是 `NT AUTHORITY/SYSTEM (S-1-5-18)`，当前用户不是它。git 2.35.2+ 因 CVE-2022-24765 的防护，在「仓库属主 ≠ 当前用户」时拒绝操作。与「工作区从 Linux 复制过来」间接相关：Linux 的 uid/gid 属主没法带到 NTFS，复制后目录 Owner 落到了 SYSTEM。

**处置**：加入全局白名单（git 官方推荐做法）

```bash
git config --global --add safe.directory D:/Desktop/Code/SSHFleet
```

撤销：`git config --global --unset safe.directory D:/Desktop/Code/SSHFleet`。

更彻底的一条路是改目录属主（`takeown` / `icacls`），没必要，未做。

---

## Comments

- 2026-10-17：本轮完成清理。主要动作：
  - 8 份 `spec-已闭环.md` → `spec.md`；2 份工单名去掉 `-已闭环` 后缀
  - `Status` 值统一为 `done`（保留原有的日期与提交号括注）。按 `git diff` 统计旧 `Status:` 行共 **59** 处；此外还有若干文件原本没有 `Status:` 行，一并补齐
  - 新建 `defect-scan-2026-10-17/`（代码缺陷集中登记）、`configurable-interpreters/spec.md`、`answer-mismatch-fill/spec.md`、`progress-update-lag/spec.md`
  - 归并删除：`win10-ansi-color/review-2026-10-16.md`（并入工单 01 Comments）、`result-verdict/recheck.md`（并入 spec Comments）、`architecture-review-2026-10-06/deferred.md`（并入 spec 第六节）
  - 删除过期交接稿：`configurable-interpreters/handoff-2026-10-07.md`、`configurable-interpreters/audit-handoff.md`、`win10-ansi-color/handoff.md`
  - 删除重复：`rewrite-to-go/open-questions.md`（Q4→D43、Q5→D44 均已在 spec 里，**Q6 原先漏了**，2026-10-17 补记 D52「检查顺序无关」；原文的证据与实测细节未并入，只保留了结论）、`rewrite-to-go/plan-coverage-已闭环.md`（并入 spec 第七节）
  - 建 ADR-0018（记录 A3 取样点提前的取舍）
  - 约束文件更新：`triage-labels.md` 新增 `done`；`issue-tracker.md` 写明三类证据底本与其他自造文件名不认
  - `lower-onboarding-cost/copy.md`、`output-audit.md` 保留为证据底本，补 `Status` 行
- 2026-10-17：原 `handoff-2026-10-08.md` 的任务书内容要点已摘要进本 spec 的「补充说明」，原文件删除。
- 2026-10-17（同日，开发者指示）：审计类文档删除。**准确范围**（经改动审计更正）：
  - 删除 `docs/issues/security-audit/` 原有 4 份文件（`HANDOFF.md`、`final-report.md`、`live-report.md`、`static-report.md`）。该目录**原本就没有 `spec.md`、也没有 `issues/`**——那些是本次整改中途新建、随后又依开发者指示删除的，不是原有内容。
  - 删除 `docs/issues/configurable-interpreters/issues/01-audit-interpreter-batch.md`。该目录同样是本次新建、随后删除，HEAD 里原本没有。
  - **代价（据实记录）**：删 `security-audit` 时只把 S1/S2/S3 摘要进 `defect-scan-2026-10-17/`，S4–S7 四条设计边界与实测原文未随之保留。**后经审计指出，S4–S7 已从 `ce618f1` 取回并单列**（见 `defect-scan-2026-10-17/spec.md` 的「设计边界」一节）；实测环境搭建过程与终端实测原文仍未取回，需要时按同一提交取。
  - `defect-scan-2026-10-17/` 保留，其中的危险命令检测两条由**该议题**承载证据与复现方式。

- 2026-10-17（收尾）：**交付证据**。

  ```text
  $ git diff --stat
   98 files changed, 285 insertions(+), 4883 deletions(-)

  $ git status --short   # 未跟踪的新文件
   ?? docs/adr/0018-accept-earlier-stats-sampling-point.md
   ?? docs/issues/answer-mismatch-fill/spec.md
   ?? docs/issues/backlog-cleanup/spec.md
   ?? docs/issues/category-stats-layout/spec.md
   ?? docs/issues/configurable-interpreters/{spec.md, findings-2026-10-07.md}
   ?? docs/issues/defect-scan-2026-10-17/
   ?? docs/issues/error-classification/spec.md
   ?? docs/issues/lower-onboarding-cost/spec.md
   ?? docs/issues/mode-centralization/{spec.md, issues/04-closeout-and-verify.md}
   ?? docs/issues/progress-update-lag/spec.md
   ?? docs/issues/remote-interaction/spec.md
   ?? docs/issues/remote-password-change/spec.md
   ?? docs/issues/result-verdict/spec.md
   ?? docs/issues/rewrite-to-go/{spec.md, issues/29-upload-structure.md}
  ```

  **收口核对**（逐项跑过）：

  ```text
  议题目录            17 个，全部有 spec.md          ✓
  非法文件名（已闭环等） 0 个                          ✓
  Status 非法值        0 个（done 78 / needs-triage 3） ✓
  一个文件多个 Status  0 处                            ✓
  控制字符（U+000C 等） 0 处                           ✓
  章节引用失准          0 处（规范章号已全部改引节名）   ✓
  ```

  **删除的文件均留了去向**：`review-2026-10-16.md`→工单 01、`recheck.md`→spec Comments、`deferred.md`→spec 第六节、`plan-coverage-已闭环.md`→spec 第七节、`backlog-2026-10-13/findings.md`→`defect-scan-2026-10-17/findings-2026-10-13.md`、`open-questions.md`→spec D43/D44（Q6 另补 D52）。

- 2026-10-17（收口后补记）：上条收口核对里的三个数（17 个目录 / 81 份文档 / done 78 + needs-triage 3）是**收口当时**的快照。其后 `defect-scan-2026-10-17/spec.md` 的 D2 拆出为独立议题 `yes-mode-danger-visible/`，故目录数由 16 增至 17、`needs-triage` 由 2 增至 3。**下次核数请以实际为准，别照抄本节。**