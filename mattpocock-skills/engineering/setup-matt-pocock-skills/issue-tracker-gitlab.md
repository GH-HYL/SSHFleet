# 问题追踪器：GitLab

本仓库的 issue 与规格以 GitLab issue 的形式存在。所有操作都使用 [`glab`](https://gitlab.com/gitlab-org/cli) CLI。

## 约定

- **创建 issue**：`glab issue create --title "..." --description "..."`。多行描述用 heredoc。传 `--description -` 可打开编辑器。
- **读取 issue**：`glab issue view <number> --comments`。机器可读输出用 `-F json`。
- **列出 issue**：`glab issue list -O json`，配合合适的 `--label` 过滤条件。
- **评论 issue**：`glab issue note <number> --message "..."`。GitLab 把评论称作「note」。
- **添加 / 移除标签**：`glab issue update <number> --label "..."` / `--unlabel "..."`。多个标签可用逗号分隔，也可重复该选项。
- **关闭**：`glab issue close <number>`。`glab issue close` 不接受关闭评论，所以先用 `glab issue note <number> --message "..."` 发出说明，再关闭。
- **合并请求（Merge requests）**：GitLab 把 PR 称作「merge request」。使用 `glab mr create`、`glab mr view`、`glab mr note` 等，形状与 `gh pr ...` 相同，只是用 `mr` 取代 `pr`、用 `note`/`--message` 取代 `comment`/`--body`。

从 `git remote -v` 推断仓库；在克隆内运行时 `glab` 会自动做这件事。

## 把合并请求当作分诊入口

**MR 作为请求入口：否。** _（如果本仓库把外部合并请求当作功能请求处理，就设为 `yes`；`/triage` 会读取这个开关。）_

设为 `yes` 时，MR 会走与 issue 相同的标签和状态，使用对应的 `glab mr` 命令：

- **读取一个 MR**：`glab mr view <number> --comments`，diff 用 `glab mr diff <number>`。
- **列出待分诊的外部 MR**：`glab mr list -F json`，然后只保留作者不是项目成员/所有者的 MR（即贡献者提交的 MR，而不是维护者正在进行的开发）。
- **评论 / 打标签 / 关闭**：`glab mr note`、`glab mr update --label`/`--unlabel`、`glab mr close`。

与 GitHub 不同，GitLab 对 issue 和 MR 分别编号，所以一旦你知道维护者指的是哪个入口，`#42` 就是无歧义的。

## 当技能说「发布到问题追踪器」时

创建一个 GitLab issue。

## 当技能说「取回相关工单」时

运行 `glab issue view <number> --comments`。

## 寻路操作（Wayfinding operations）

由 `/wayfinder` 使用。**地图（map）** 是一个单独的 issue，其**子（child）** issue 就是工单（ticket）。

- **地图**：一个标记为 `wayfinder:map` 的 issue，承载 Notes / Decisions-so-far / Fog 正文。`glab issue create --label wayfinder:map`。（在原生存史诗（epic）的 GitLab 付费档位上，可以用史诗承载地图；而带标签的 issue 在任何档位都可用。）
- **子工单**：一个在描述顶部带有 `Part of #<map>`、并带 `wayfinder:<type>` 标签（`research`/`prototype`/`grilling`/`task`）的 issue。一旦被认领，工单就指派给驱动它的开发者。
- **阻塞（Blocking）**：GitLab 的**原生阻塞链接（native blocking link）**，这是标准且 UI 可见的表示方式。用 `/blocked_by #<n>` 快捷操作添加，以 note 的形式发出（`glab issue note <child> --message "/blocked_by #<blocker>"`）。原生阻塞链接是 Premium/Ultimate 功能；在免费档位（或不可用处）退回到在描述顶部写一行 `Blocked by: #<n>, #<n>`。当每个阻塞者都关闭时，工单即解除阻塞。
- **前沿查询（Frontier query）**：`glab issue list -O json`，范围限定为地图的子项，剔除任何存在未关闭阻塞者的项：指向未关闭 issue 的原生 `blocked_by` 链接（`glab api projects/:id/issues/<child-iid>/links`），或 `Blocked by` 行里有未关闭 issue，或有指派人；按地图顺序，最先的胜出。
- **认领（Claim）**：`glab issue update <n> --assignee @me`，这是本会话的第一次写入。
- **解决（Resolve）**：`glab issue note <n> --message "<answer>"`，然后 `glab issue close <n>`，再往地图的 Decisions-so-far 追加一条上下文指针（要点 + 链接）。
