# 范围之外知识库（Out-of-Scope Knowledge Base）

仓库中的 `.out-of-scope/` 目录存放被否决的功能请求的长期记录。它有两个用途：

1. **组织记忆（Institutional memory）**：某个功能为什么被否决，好让 issue 关闭后理由不会丢失
2. **去重（Deduplication）**：当新来的 issue 与此前的否决相匹配时，技能可以浮出先前的决定，而不是重新争论一遍

## 目录结构

```
.out-of-scope/
├── dark-mode.md
├── plugin-system.md
└── graphql-api.md
```

每个**概念**一个文件，而不是每个 issue 一个文件。请求同一件事的多个 issue 归入同一个文件。

## 文件格式

文件应当写得松弛易读，更像一篇简短的设计文档，而不是数据库条目。用段落、代码示例和例子把理由讲清楚，让第一次读到它的人也能用得上。

```markdown
# Dark Mode

This project does not support dark mode or user-facing theming.

## Why this is out of scope

The rendering pipeline assumes a single color palette defined in
`ThemeConfig`. Supporting multiple themes would require:

- A theme context provider wrapping the entire component tree
- Per-component theme-aware style resolution
- A persistence layer for user theme preferences

This is a significant architectural change that doesn't align with the
project's focus on content authoring. Theming is a concern for downstream
consumers who embed or redistribute the output.

```ts
// The current ThemeConfig interface is not designed for runtime switching:
interface ThemeConfig {
  colors: ColorPalette; // single palette, resolved at build time
  fonts: FontStack;
}
```

## Prior requests

- #42: "Add dark mode support"
- #87: "Night theme for accessibility"
- #134: "Dark theme option"
```

### 给文件命名

用简短、有描述性的 kebab-case 名称给概念命名：`dark-mode.md`、`plugin-system.md`、`graphql-api.md`。这个名字应当足够可辨识，让人浏览目录时不打开文件也能明白什么被否决了。

### 写理由

理由应当有实质内容：不是「我们不想要这个」，而是为什么。好的理由会引用：

- 项目范围或理念（「本项目专注于 X；主题化是下游的事」）
- 技术约束（「支持它会需要 Y，而 Y 与我们的 Z 架构冲突」）
- 战略决策（「我们选择用 A 而不是 B，因为……」）

理由应当耐久。避免引用临时境况（「我们眼下太忙了」）；那些不是真正的否决，只是推迟。

## 何时检查 `.out-of-scope/`

分诊期间（第 1 步：收集上下文），读取 `.out-of-scope/` 中的所有文件。评估一个新 issue 时：

- 检查该请求是否与某个已有的范围之外概念匹配
- 匹配按概念相似性，而非关键词：「night theme」与 `dark-mode.md` 匹配
- 如果匹配，就把它摆给维护者：「这与 `.out-of-scope/dark-mode.md` 类似。我们之前因为[理由]否决了它。你现在还是这个看法吗？」

维护者可以：

- **确认**：新 issue 被加入已有文件的「Prior requests」列表，然后关闭
- **重新考虑**：范围之外文件被删除或更新，该 issue 走正常分诊流程
- **不同意**：这些 issue 相关但不同，走正常分诊流程

## 何时写入 `.out-of-scope/`

只有当某个 **enhancement（而非 bug）** 被作为 `wontfix` *否决* 时。这同样适用于 enhancement 类 PR 与 issue：被否决的 PR 也记录在这里，免得同一个请求又作为新代码回来。

**不要**因为某个东西是**已实现**而被作为 `wontfix` 关闭时就写到这里。那是一个已构建的功能，不是被否决的；记录它会让去重检查被虚假的否决污染。相应地，关闭评论应指出该功能已经在哪里。

流程：

1. 维护者判定某个功能请求在范围之外
2. 检查是否已存在匹配的 `.out-of-scope/` 文件
3. 如果有：把新 issue 追加到「Prior requests」列表
4. 如果没有：新建文件，包含概念名、决定、理由和第一个先前请求
5. 在该 issue 上发布评论，说明决定并提到该 `.out-of-scope/` 文件
6. 用 `wontfix` 标签关闭该 issue

## 更新或删除范围之外文件

如果维护者改变了对某个此前被否决概念的看法：

- 删除该 `.out-of-scope/` 文件
- 技能不需要重开旧 issue；它们是历史记录
- 触发这次重新考虑的新 issue 走正常分诊流程
