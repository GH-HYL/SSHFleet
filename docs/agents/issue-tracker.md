# Issue 载体：本地 Markdown

本仓库的 issue 与 spec 以 markdown 文件形式存放在 `docs/issues/` 下。

## 约定

- 一个功能一个目录：`docs/issues/<feature-slug>/`
- 规格文档：`docs/issues/<feature-slug>/spec.md`
- 实现工单**一条一个文件**：`docs/issues/<feature-slug>/issues/<NN>-<slug>.md`，从 `01` 起编号，**绝不合并成单个工单文件**
- 分诊状态记录在文件顶部附近的 `Status:` 行（角色字符串见 `triage-labels.md`）
- 评论与讨论历史追加到文件底部的 `## Comments` 标题下

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
