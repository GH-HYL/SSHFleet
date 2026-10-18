# 问题追踪器：GitHub

本仓库的 issue 与规格以 GitHub issue 的形式存在。所有操作都使用 `gh` CLI。

## 约定

- **创建 issue**：`gh issue create --title "..." --body "..."`。多行正文用 heredoc。
- **读取 issue**：`gh issue view <number> --json number,title,body,labels,comments`。
- **列出 issue**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，配合合适的 `--label` 与 `--state` 过滤条件。
- **把某个 issue 设为父 issue 的子 issue**：`gh issue create --parent <parent> ...`，或事后用 `gh issue edit <parent> --add-sub-issue <child>`（`gh` 2.94+）。更老的 `gh`：`gh api --method POST repos/<owner>/<repo>/issues/<parent>/sub_issues -F sub_issue_id=<child-db-id>`（数据库 id，见下文 **阻塞（Blocking）**）。如果不支持子 issue，就在子 issue 正文顶部写上 `Part of #<parent>`。
- **评论 issue**：`gh issue comment <number> --body "..."`
- **添加 / 移除标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **关闭**：`gh issue close <number> --comment "..."`

从 `git remote -v` 推断仓库；在克隆内运行时 `gh` 会自动做这件事。

## 把拉取请求当作分诊入口

**PR 作为请求入口：否。** _（如果本仓库把外部 PR 当作功能请求处理，就设为 `yes`；`/triage` 会读取这个开关。）_

设为 `yes` 时，PR 会走与 issue 相同的标签和状态，使用对应的 `gh pr` 命令：

- **读取一个 PR**：`gh pr view <number> --comments`，diff 用 `gh pr diff <number>`。
- **列出待分诊的外部 PR**：`gh api --paginate 'repos/{owner}/{repo}/pulls?state=open' --jq '.[] | select(.author_association | IN("OWNER","MEMBER","COLLABORATOR") | not) | {number, title, author: .user.login, author_association, labels: [.labels[].name]}'`。
- **评论 / 打标签 / 关闭**：`gh pr comment`、`gh pr edit --add-label`/`--remove-label`、`gh pr close`。

GitHub 的 issue 和 PR 共用一个编号空间，所以一个光秃秃的 `#42` 可能是两者之一：先用 `gh pr view 42` 解析，再回退到 `gh issue view 42`。

## 当技能说「发布到问题追踪器」时

创建一个 GitHub issue。

## 当技能说「取回相关工单」时

按上文 **读取 issue** 的方式读取它。

## 寻路操作（Wayfinding operations）

由 `/wayfinder` 使用。**地图（map）** 是一个单独的 issue，其**子（child）** issue 就是工单（ticket）。

- **地图**：一个标记为 `wayfinder:map` 的 issue，承载 Notes / Decisions-so-far / Fog 正文。`gh issue create --label wayfinder:map`。
- **子工单**：作为 GitHub 子 issue 链接到地图的 issue（见 **把某个 issue 设为父 issue 的子 issue**）。在未启用子 issue 的地方，把子工单加进地图正文的任务列表，并在子 issue 正文顶部写上 `Part of #<map>`。标签：`wayfinder:<type>`（`research`/`prototype`/`grilling`/`task`）。一旦被认领，工单就指派给驱动它的开发者。
- **阻塞（Blocking）**：GitHub 的**原生 issue 依赖（native issue dependencies）**，这是标准且 UI 可见的表示方式。用 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>` 添加一条边，其中 `<blocker-db-id>` 是阻塞者的数字**数据库 id**（`gh api repos/<owner>/<repo>/issues/<n> --jq .id`，_不是_ `#number` 也不是 `node_id`）。GitHub 会报告 `issue_dependencies_summary.blocked_by`（只含未关闭的阻塞者，也就是实时门禁）。在依赖不可用的地方，退回到在子 issue 正文顶部写一行 `Blocked by: #<n>, #<n>`。当每个阻塞者都关闭时，工单即解除阻塞。
- **前沿查询（Frontier query）**：列出地图的未关闭子项（`gh issue list --state open`，范围限定为地图的子 issue / 任务列表），剔除任何存在未关闭阻塞者（`issue_dependencies_summary.blocked_by > 0`，或 `Blocked by` 行里有未关闭 issue）或有指派人的项；按地图顺序，最先的胜出。
- **认领（Claim）**：`gh issue edit <n> --add-assignee @me`，这是本会话的第一次写入。
- **解决（Resolve）**：`gh issue comment <n> --body "<answer>"`，然后 `gh issue close <n>`，再往地图的 Decisions-so-far 追加一条上下文指针（要点 + 链接）。
