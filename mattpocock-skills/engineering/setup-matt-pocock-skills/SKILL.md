---
name: setup-matt-pocock-skills
description: "为工程类技能配置本仓库：设置它的问题追踪器（issue tracker）、分诊标签（triage label）词表，以及领域文档（domain doc）布局。在首次使用其他工程类技能之前运行一次。"
disable-model-invocation: true
---

# setup-matt-pocock-skills（Setup Matt Pocock's Skills）

搭建各工程类技能所假定的、按仓库分别配置的设定：

- **问题追踪器（issue tracker）**：issue 存放在哪里（默认是 GitHub；也开箱支持本地 markdown）
- **分诊标签（triage label）**：五个标准分诊角色（triage role）所使用的字符串
- **领域文档（domain docs）**：`GLOSSARY.md` 与 ADR 存放在哪里，以及阅读它们的消费规则

这是一个由提示词驱动的技能，不是确定性脚本。先探查，再呈现你发现的内容，与用户确认，然后写入。

## 流程

### 1. 探查

查看当前仓库以了解它的起始状态。凡是存在的都读一读，不要臆断：

- `git remote -v` 和 `.git/config`：这是 GitHub 仓库吗？是哪一个？
- 仓库根目录的 `AGENTS.md` 和 `CLAUDE.md`：哪一个存在？其中是否已经有 `## Agent skills` 段落？
- 仓库根目录的 `GLOSSARY.md` 和 `GLOSSARY-MAP.md`
- `docs/adr/` 以及任何 `src/*/docs/adr/` 目录
- `docs/agents/`：本技能此前的产出是否已经存在？
- `.scratch/`：这是本地 markdown 问题追踪器约定已在使用的迹象
- `triage` 技能是否已安装？（与本技能并列的 `triage` 技能文件夹，或你的可用技能列表中有 `triage`。）这决定 B 段是否要运行。
- 单体仓库（monorepo）信号：`pnpm-workspace.yaml`、`package.json` 中的 `workspaces` 字段，或带有自己 `src/` 的、内容充实的 `packages/*`。这些只在真正大型的多包仓库里出现；没有它们就意味着单上下文（single-context），而几乎所有仓库都是这样。

### 2. 呈现发现并提问

概述哪些已存在、哪些缺失。然后按顺序逐段处理。一段一个问题，答完再进下一段。

每段都先给出推荐答案，好让用户用一个词就能接受。只有当选择确实出现分叉时才给一行说明；若探查已经确定了答案，就整段跳过（未安装 `triage` 时跳过 B 段，没有 monorepo 时跳过 C 段）。

**A 段：问题追踪器。**

> 说明：「问题追踪器」就是这个仓库里 issue 存放的地方。像 `to-tickets`、`triage` 和 `to-spec` 这样的技能会读写它。它们需要知道该调用 `gh issue create`、在 `.scratch/` 下写一个 markdown 文件，还是遵循你描述的其他工作流。请选你实际为本仓库跟踪工作的地方。

默认立场：这些技能是为 GitHub 设计的。如果某个 `git remote` 指向 GitHub，就提议 GitHub。如果某个 `git remote` 指向 GitLab（`gitlab.com` 或自托管主机），就提议 GitLab。否则（或者用户更偏好别的），提供以下选项：

- **GitHub**：issue 存放在仓库的 GitHub Issues 中（使用 `gh` CLI）
- **GitLab**：issue 存放在仓库的 GitLab Issues 中（使用 [`glab`](https://gitlab.com/gitlab-org/cli) CLI）
- **本地 markdown（Local markdown）**：issue 以文件形式存放在本仓库的 `.scratch/<feature>/` 下（适合个人项目或没有远程仓库的项目）
- **其他（Other）**（Jira、Linear 等）：请用户用一段话描述工作流；技能会把它记为自由形式的散文描述

把选择记录到 `docs/agents/issue-tracker.md`。GitHub 和 GitLab 模板带有「PR 作为请求入口（PRs as a request surface）」开关，默认**关闭**。保持关闭，不要提起它：想要把外部 PR 纳入分诊队列的用户，之后可以自己在文件里翻这个开关。

**B 段：分诊标签词表。** 如果 `triage` 技能没安装，就整段跳过（探查已告诉你），因为未安装的技能不需要标签。

如果已安装，就只问一个问题：

> 你想保留默认的分诊标签吗？（推荐：**是**）

默认值就是五个标准角色，每个标签字符串与其角色名相同：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。答**是**，就照原样写入。只有当用户说不——通常是因为他们的追踪器已经在用别的名字（例如用 `bug:triage` 代替 `needs-triage`）——才收集这些替换项，好让 `triage` 应用已有的标签，而不是创建重复标签。

**C 段：领域文档。** 默认**单上下文**（仓库根目录一份 `GLOSSARY.md` + `docs/adr/`）。这适合几乎所有仓库；直接写入，不必询问。

只有当探查发现 monorepo 信号时，才提供**多上下文（multi-context）**选项（根目录的 `GLOSSARY-MAP.md` 指向各上下文的 `GLOSSARY.md` 文件）。然后确认他们想要哪种布局。

### 3. 确认并编辑

向用户展示草稿：

- 要加到正在编辑的那个 `CLAUDE.md` / `AGENTS.md` 里的 `## Agent skills` 区块（选择规则见第 4 步）
- `docs/agents/issue-tracker.md`、`docs/agents/domain.md` 和 `docs/agents/triage-labels.md` 的内容（最后这个只在 `triage` 已安装时才有）

让他们在写入之前先做修改。

### 4. 写入

**选定要编辑的文件：**

- 如果 `CLAUDE.md` 存在，就编辑它。
- 否则如果 `AGENTS.md` 存在，就编辑它。
- 如果两者都不存在，问用户要创建哪一个；不要替他们选。

当 `CLAUDE.md` 已存在时，绝不创建 `AGENTS.md`（反之亦然）；始终编辑已经存在的那个。

如果所选文件中已经存在 `## Agent skills` 区块，就原地更新其内容，而不是追加一个重复区块。不要覆盖用户对周边段落的修改。

区块如下：

```markdown
## Agent skills

### Issue tracker

[issue 存放在哪里的一行摘要]。见 `docs/agents/issue-tracker.md`。

### Triage labels

[标签词表的一行摘要]。见 `docs/agents/triage-labels.md`。

### Domain docs

[布局的一行摘要："single-context" 或 "multi-context"]。见 `docs/agents/domain.md`。
```

只有当 `triage` 已安装且 B 段运行过时，才包含 `### Triage labels` 子区块并写入 `docs/agents/triage-labels.md`。若未安装，二者都省略。

当 B 段在 GitHub 或 GitLab 上运行过时，为追踪器缺失的每个已配置标签创建标签（`gh label create` / `glab label create`）。

然后以本技能文件夹中的种子模板为起点写入这些文档文件：

- [issue-tracker-github.md](./issue-tracker-github.md)：GitHub 问题追踪器
- [issue-tracker-gitlab.md](./issue-tracker-gitlab.md)：GitLab 问题追踪器
- [issue-tracker-local.md](./issue-tracker-local.md)：本地 markdown 问题追踪器
- [triage-labels.md](./triage-labels.md)：标签映射（仅在 `triage` 已安装时）
- [domain.md](./domain.md)：领域文档的消费规则 + 布局

对于「其他」问题追踪器，用用户的描述从零写 `docs/agents/issue-tracker.md`。

### 5. 完成

告诉用户配置已完成，以及哪些工程类技能现在会读取这些文件。提一句他们之后可以直接编辑 `docs/agents/*.md`；只有想切换问题追踪器或从头重来时才需要重跑本技能。
