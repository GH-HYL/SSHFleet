# 问题追踪器：本地 Markdown

本仓库的 issue 与规格以 markdown 文件的形式存放在 `.scratch/` 中。

## 约定

- 每个功能一个目录：`.scratch/<feature-slug>/`
- 规格是 `.scratch/<feature-slug>/spec.md`
- 实现类 issue 是每个工单一个文件，位于 `.scratch/<feature-slug>/issues/<NN>-<slug>.md`，从 `01` 开始编号，绝不合并成单个工单文件
- 分诊状态记为每个 issue 文件顶部附近的 `Status:` 行（角色字符串见 `triage-labels.md`）
- 评论与讨论历史追加到文件底部 `## Comments` 标题下

## 当技能说「发布到问题追踪器」时

在 `.scratch/<feature-slug>/` 下新建一个文件（需要的话先创建目录）。

## 当技能说「取回相关工单」时

读取被引用路径上的文件。用户通常会直接给出路径或 issue 编号。

## 寻路操作（Wayfinding operations）

由 `/wayfinder` 使用。**地图（map）** 是一个文件，每个**子（child）**工单对应一个文件。

- **地图**：`.scratch/<effort>/map.md`（承载 Notes / Decisions-so-far / Fog 正文）。
- **子工单**：`.scratch/<effort>/issues/NN-<slug>.md`，从 `01` 开始编号，正文中写明问题。`Type:` 行记录工单类型（`research`/`prototype`/`grilling`/`task`）；`Status:` 行记录 `claimed`/`resolved`。
- **阻塞（Blocking）**：顶部附近的一行 `Blocked by: NN, NN`。当它列出的每个文件都是 `resolved` 时，工单即解除阻塞。
- **前沿（Frontier）**：扫描 `.scratch/<effort>/issues/`，找出未关闭、未被阻塞且未被认领的文件；编号最小的胜出。
- **认领（Claim）**：在任何工作开始之前先设置 `Status: claimed` 并保存。
- **解决（Resolve）**：在 `## Answer` 标题下追加答案，设置 `Status: resolved`，再往 `map.md` 中地图的 Decisions-so-far 追加一条上下文指针（要点 + 链接）。
