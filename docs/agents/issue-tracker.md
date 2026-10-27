# Issue 载体：本地 Markdown

本仓库的 issue 与 spec 以 markdown 文件形式存放在 `docs/issues/` 下。

## 约定

- 一个功能一个目录：`docs/issues/<feature-slug>/`
- 规格文档：`docs/issues/<feature-slug>/spec.md`
- 实现工单**一条一个文件**：`docs/issues/<feature-slug>/issues/<NN>-<slug>.md`，从 `01` 起编号，**绝不合并成单个工单文件**
- 分诊状态记录在文件顶部附近的 `Status:` 行（角色字符串见 `triage-labels.md`）
- 评论与讨论历史追加到文件底部的 `## Comments` 标题下
- **文件名只承载 slug，不承载状态**：一律写 `spec.md`，状态写在 `Status:` 行

## 议题内的其他文件（本仓库扩充）

技能只定义 `spec.md`、`issues/NN-*.md`、`map.md` 三种文件。本仓库另有几类文件会出现在议题目录里，各有明确职责，**不是可以随意新建的杂项**：

| 文件 | 职责 | 归属 |
| --- | --- | --- |
| `spec.md` | 这件事要做成什么样 | 作者维护 |
| `issues/NN-*.md` | 一条可执行工单 | 作者维护 |
| `map.md` | 仅 `/wayfinder`，承载探路记录 | 作者维护 |

**另有三类「证据底本」，允许单独成文件**——它们承载的是**做判断所依据的原始材料**，合并进工单的 `## Comments` 会丢掉「这些结论是怎么来的」：

| 文件 | 什么时候才允许存在 | 判据 |
| --- | --- | --- |
| `findings-<日期>.md` | 排查阶段的**根因分析**，含已证否的方向 | 比工单大一个量级；工单只写「做了什么」，不写「排除了什么」。**无日期版的 `findings.md` 不再认**（本仓库不用它） |
| `audit.md` | **实测报告**，是若干条工单的事实底本 | 多条工单共用同一份证据时。撤掉了工单对应关系的旧审计报告，按 `findings-<日期>.md` 处理 |
| `<something>-copy.md` | **文案底本**，实施后要回写 | 它具有「随每次实施更新」的维护义务，`## Comments` 是只追加的事件流，承接不了。**退役后不再回写的，按历史记录处理**——补 `Status:` 说明退役，或并入 `## Comments` |

**判据只有一句**：这份材料是「某次做完了的记录」，还是「后来的人做判断要依据的东西」？前者归 `## Comments`，后者才允许独立成文件。

**其他自造文件名一律不认**：`handoff*`、`recheck`、`deferred`、`open-questions`、`plan-coverage`、`review-*`、`final-report`、`*-handoff` 等都不是载体里的一类——
- 交接背景已被后续结论取代的，**直接删**
- 验收或复跑记录，**归进对应工单的 `## Comments`**
- 「明知有代价、选择不修」的裁定，**归进 ADR**（写清代价是什么、为什么不修）

> **授权记录**：以上三类证据底本的白名单与「其他自造文件名一律不认」的黑名单，由作者 2026-10-17 选定「保留为独立文件，在载体约定里给名分」。本条属对约束类文件的实质修改，按「一、约束·1·2」记录下来源。

## 当某个技能要求「发布到 issue 载体」时

在 `docs/issues/<feature-slug>/` 下新建文件（目录不存在则创建）。

## 当某个技能要求「取回相关工单」时

读取引用路径对应的文件。用户通常会直接给出路径或工单编号。

## Wayfinding 操作

供 `/wayfinder` 使用。**地图**是一个文件，每条工单对应一个**子**文件。

- **地图**：`docs/issues/<effort>/map.md`，承载 Notes / Decisions-so-far / Fog 正文。
- **子工单**：`docs/issues/<effort>/issues/NN-<slug>.md`，从 `01` 起编号，问题写在正文中。`Type:` 行记录工单类型（`research`/`prototype`/`grilling`/`task`）；`Status:` 行记录 `claimed`/`resolved`。
- **阻塞**：顶部附近的 `Blocked by: NN, NN` 行。当它列出的每个文件都是 `resolved` 时，该工单即解除阻塞。
- **前沿（Frontier）**：扫描 `docs/issues/<effort>/issues/`，取未关闭、未阻塞、未被认领的文件，编号最小的优先。
- **认领**：先把 `Status:` 置为 `claimed` 并保存，再开始任何实际工作。
- **结算**：在 `## Answer` 标题下追加答案，把 `Status:` 置为 `resolved`，然后把一条上下文指针（要点 + 链接）追加到 `map.md` 的 Decisions-so-far。
