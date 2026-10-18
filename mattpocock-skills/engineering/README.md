# 工程技能

我日常写代码时用的技能。

## 用户调用（User-invoked）

只有你手动敲出来才能触达（Claude Code：`disable-model-invocation: true`；Codex：`agents/openai.yaml` 里的 `policy.allow_implicit_invocation: false`）。

- **[ask-matt](./ask-matt/SKILL.md)**：问一问哪个技能或流程适合你当下的处境。它是本仓库中用户调用技能之上的一个路由技能。
- **[grill-with-docs](./grill-with-docs/SKILL.md)**：拷问（grilling）会话，同时构建你项目的领域模型，当场打磨术语并更新 `GLOSSARY.md` 和 ADR。
- **[triage](./triage/SKILL.md)**：让 issue 在一套分诊角色的状态机（state machine）中流转。
- **[improve-codebase-architecture](./improve-codebase-architecture/SKILL.md)**：扫描代码库找出深化机会，以可视化 HTML 报告的形式呈现，再就你选中的那一个展开拷问。
- **[setup-matt-pocock-skills](./setup-matt-pocock-skills/SKILL.md)**：为本仓库配置这些工程技能所需的东西（问题追踪器、分诊标签、领域文档布局）。每个仓库跑一次。
- **[to-spec](./to-spec/SKILL.md)**：把当前对话变成一份规格（spec）并发布到问题追踪器。
- **[to-tickets](./to-tickets/SKILL.md)**：把任何计划、规格或对话拆成一组曳光弹工单（tracer-bullet ticket），每张工单都声明它的阻塞边（blocking edge）——要么写成本地文件里的文字，要么做成真实追踪器上的原生阻塞链接。
- **[implement](./implement/SKILL.md)**：实现规格或一组工单所描述的工作，在预先商定的接缝（seam）上驱动 `/tdd`，提交前用 `/code-review` 收尾。
- **[implement-spec](./implement-spec/SKILL.md)**：在一个集成分支（integration branch）上实现整份规格。把工单当作任务图（task graph）来推进，跨就绪前沿（ready frontier）并行跑实现者子 agent 以获得最大并发，最后用 `/code-review` 收尾。
- **[wayfinder](./wayfinder/SKILL.md)**：把一大块工作（多到超出单个 agent 会话容量）规划成问题追踪器上一张共享的决策工单地图，一次解决一个，直到通往目的地的路清晰起来。
- **[retro](./retro/SKILL.md)**：在一次会话之后，为编码 agent 的环境（导航、自动化检查、编码标准、引导文件、工具链）提出改进建议，最严重的排前面。

## 模型调用（Model-invoked）

模型或用户皆可触达（触发语写得比较丰富，以便模型能够主动够到它们）。

- **[prototype](./prototype/SKILL.md)**：构建一个用完即弃的原型来回答某个设计问题：针对状态／逻辑是一个可分享的 HTML 文件，针对 UI 则是几个可切换的变体。

- **[diagnosing-bugs](./diagnosing-bugs/SKILL.md)**：针对疑难 bug 和性能回归的有纪律的诊断回路：先建一条会在这个 bug 上变红的反馈回路（feedback loop）→ 最小化 → 提出假设 → 埋点观测 → 修复 → 回归测试（regression test）。
- **[research](./research/SKILL.md)**：对照高可信度的一手来源（primary sources）调研某个问题，并把结论以带引用的 Markdown 文件形式留在仓库里，作为后台 agent 运行。
- **[tdd](./tdd/SKILL.md)**：测试驱动开发，采用红-绿-重构（red-green-refactor）循环。一次一个垂直切片（vertical slice）地构建功能或修复 bug。
- **[domain-modeling](./domain-modeling/SKILL.md)**：通过质疑术语、用场景施压、当场更新 `GLOSSARY.md` 和 ADR，主动构建并打磨项目的领域模型。
- **[codebase-design](./codebase-design/SKILL.md)**：设计深模块（deep module）所需的共享纪律与词汇：小接口、干净的接缝、通过接口即可测试。
- **[code-review](./code-review/SKILL.md)**：针对自某个固定点以来 diff 的双轴复查：**标准（Standards）**（代码是否遵循本仓库成文的编码标准，外加一份 Fowler 坏味道基线？）与**规格（Spec）**（代码是否忠实实现了源头的 issue／规格？），以并行子 agent 的方式运行。
- **[pr](./pr/SKILL.md)**：一份 pull request 正文应有的形态：用能说清改动的最小可视化呈现的摘要、证明其有效的改动前后证据，以及一次合并危险度判断（单向门还是双向门，外加影响半径）。
- **[wizard](./wizard/SKILL.md)**：生成一个交互式 bash 向导（wizard），引导人走完只有他们才能执行的步骤：资源开通（provisioning）基础设施、配置凭证或 CI 密钥、在陌生的第三方控制台里点来点去、跑一次性的迁移或切换上线（cutover）。
