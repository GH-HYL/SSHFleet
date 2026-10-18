# Productivity（生产力）

通用工作流工具，不针对代码。

## User-invoked（用户调用）

只有你亲手打出它们才能触发（Claude Code：`disable-model-invocation: true`；Codex：`agents/openai.yaml` 里的 `policy.allow_implicit_invocation: false`）。

- **[grill-me](./grill-me/SKILL.md)**：就一个方案或设计接受毫不留情的访谈，直到设计树的每一条分支都被解决。
- **[handoff](./handoff/SKILL.md)**：把当前对话压缩成一份交接文档，让另一个 agent 能接着干。
- **[teach](./teach/SKILL.md)**：跨多次会话教用户一项新技能或概念，用当前目录作为有状态的教学工作区。
- **[to-questionnaire](./to-questionnaire/SKILL.md)**：把一个你独自答不了的决策，变成一份 Markdown 问卷，交给唯一能回答的那个人（异步填，或者会上一起填）。
- **[wait-what](./wait-what/SKILL.md)**：一条消息没听懂的那一刻就发它。agent 会用你缺的那部分上下文、用大白话、并用你 `GLOSSARY.md` 里的词汇重新讲一遍。

## Model-invoked（模型调用）

模型或用户都能触发（触发措辞写得丰富，模型才够得着）。

- **[grilling](./grilling/SKILL.md)**：就一个方案、决策或想法对用户进行毫不留情的访谈，直到设计树的每一条分支都被解决。
- **[writing-for-agents](./writing-for-agents/SKILL.md)**：为 agent 写文档：技能、AGENTS.md/CLAUDE.md，以及任何 agent 通过指针抵达的文档。
