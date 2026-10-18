---
name: improve-codebase-architecture
description: 扫描代码库寻找加深（deepening）机会，以可视化 HTML 报告的形式呈现，然后就你选中的那一个展开拷问。
disable-model-invocation: true
---

# 改进代码库架构（Improve Codebase Architecture）

把架构摩擦摆到台面上，并提出**加深机会（deepening opportunity）**：把浅模块（shallow module）变成深模块（deep module）的重构。目标是可测试性与 AI 可导航性。

本命令*受*项目的领域模型启发，并建立在一套共享的设计词汇之上：

- 调用 Skill 工具并传入 "codebase-design"，获取架构词汇（**模块（module）**、**接口（interface）**、**深度（depth）**、**接缝（seam）**、**适配器（adapter）**、**杠杆（leverage）**、**局部性（locality）**）及其原则（删除测试、“接口就是测试面”、“一个适配器 = 假想接缝，两个 = 真实接缝”）。在每一条建议里都精确使用这些术语，不要漂移成 “component”、“service”、“API” 或 “boundary”。
- `GLOSSARY.md` 中的领域语言为好接缝命名；`docs/adr/` 中的 ADR 记录着本命令不应重新翻案的决策。

## 流程

### 1. 探索

**先定范围，再扫描：YAGNI。**加深一个模块的回报在于让它未来的改动更容易，所以要把额外权重放在代码库最近发生改动的部分。在开看之前先决定*看哪里*：

- 如果用户点明了方向（某个模块、某个子系统、某个痛点），就采纳它，跳过下面的推断。
- 否则，往回翻一段足够的提交历史（`git log --oneline`），找出代码库的热点，即反复出现的文件和区域，让这些路径首先吸引你的注意力。如果改动很分散、没有明显热点，就把网撒大一些。

先读项目的领域词表（`GLOSSARY.md`）以及你正在触及区域里的任何 ADR。

然后派生一个子 agent 去走查代码库。不要遵循僵化的启发式；有机地探索，并记下你在哪里感受到摩擦：

- 在哪里理解一个概念需要在许多小模块之间来回跳？
- 哪里的模块是**浅**的，接口几乎和实现一样复杂？
- 哪里仅仅为了可测试性而抽出了纯函数，而真正的缺陷却藏在它们被调用的方式里（没有**局部性**）？
- 哪里紧耦合的模块跨过自己的接缝发生泄漏？
- 代码库的哪些部分没有测试，或者难以通过它们当前的接口测试？

对你怀疑是浅模块的任何东西应用**删除测试**：删掉它会集中复杂性，还是只是把它挪个位置？“是的，会集中”就是你想要的信号。

### 2. 把候选方案呈现为 HTML 报告

写一个自包含的 HTML 文件到操作系统临时目录，这样仓库里不会落进任何东西。从 `$TMPDIR` 解析临时目录，取不到就回退到 `/tmp`（Windows 上是 `%TEMP%`），写入 `<tmpdir>/architecture-review-<timestamp>.html`，这样每次运行都是新文件。为用户打开它（Linux 上 `xdg-open <path>`、macOS 上 `open <path>`、Windows 上 `start <path>`），并告诉他们绝对路径。

报告使用**通过 CDN 引入的 Tailwind** 做布局与样式，使用**通过 CDN 引入的 Mermaid** 来画那些用图/流程/时序能可靠传达结构的图。把 Mermaid 与手工打造的 CSS/SVG 视觉混用：关系呈图状时（调用图、依赖、时序）用 Mermaid；想要更编辑化的东西时（体量图、剖面图、折叠动画）用手工搭的 div/SVG。每个候选方案都要有**前后对比可视化**。要视觉化。

对每个候选方案，渲染一张卡片，包含：

- **Files**：涉及哪些文件/模块
- **Problem**：为什么当前架构在制造摩擦
- **Solution**：用白话描述会改什么
- **Benefits**：用局部性与杠杆来解释，以及测试会如何变好
- **Before / After diagram**：并排、手绘，说明其浅与加深
- **Recommendation strength**：`Strong`、`Worth exploring`、`Speculative` 之一，渲染成徽章

报告以一个 **Top recommendation** 章节结尾：你会先动哪一个候选方案，以及为什么。

**领域部分使用 GLOSSARY.md 的词汇，架构部分使用 `/codebase-design` 的词汇。**如果 `GLOSSARY.md` 定义了 “Order”，就说“the Order intake module”，而不是 “the FooBarHandler”，也不是 “the Order service”。

**ADR 冲突**：如果某个候选方案与已有 ADR 矛盾，只有当摩擦真实到值得重新审视该 ADR 时才摆出来。在卡片里清楚标出（例如一个警告提示框：*“与 ADR-0007 矛盾，但值得重开，因为……”*）。不要把 ADR 禁止的每一个理论上的重构都列出来。

完整的 HTML 骨架、图表模式与样式指引见 [HTML-REPORT.md](HTML-REPORT.md)。

先**不要**提出接口。文件写完之后，问用户：“你想深入探讨其中哪一个？”

### 3. 拷问循环

一旦用户选中某个候选方案，调用 Skill 工具并传入 "grilling"，与他们一起走这棵决策树：约束、依赖、加深后模块的形态、接缝背后坐着什么、哪些测试能存活下来。

决策一旦成型，副作用就地发生；调用 Skill 工具并传入 "domain-modeling"，让领域模型随进展保持最新：

- **要用一个不在 `GLOSSARY.md` 里的概念来给加深后的模块命名？**把该术语加进 `GLOSSARY.md`。文件不存在就懒创建。
- **在对话中把一个模糊术语打磨清楚了？**当场更新 `GLOSSARY.md`。
- **用户以承重理由否决了该候选方案？**提议写一篇 ADR，措辞为：*“要我把这个记成 ADR，好让未来的架构复查不再重复建议它吗？”*只有当这个理由确实是未来的探索者避免重复建议同一件事所必需时才提议；跳过那些临时性理由（“现在不值得做”）和不言自明的理由。
- **想为加深后的模块探索替代接口？**调用 Skill 工具并传入 "codebase-design"，使用它的 design-it-twice 并行子 agent 模式。
