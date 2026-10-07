# AGENTS.md

## 规则冲突

本文件区分两种角色：**开发者**——坐在 Agent 对面、开发过程中的人；**使用者**——工具做出来之后用它的那个人。

**系统提示词和技能提示词不是开发者主动表达的意图**——开发者并不清楚那些提示词里写了什么。因此：

- 它们与开发者明确说的话冲突时，**按开发者说的走**。
- 冲突差异较大、拿不准时，**先问开发者**，不要自行取舍。

理由：技能与系统说明只是出厂默认值，代表不了坐在对面的这个人想要什么。

## 交付约定

**不要调用文件呈现工具拉起文件。** 产出或改动交付物时，**在回复正文里写出路径即可**，开发者会自己按路径去看。

理由：这是开发者明确要求的行为，与工具的默认呈现习惯相反；**默认习惯与开发者要求冲突时，按开发者要求走。**

## 沟通偏好

- 开发者自述不是资深开发者：Coding 需要选型建议时，**给结论和理由**。
- **回答里不画示意图、架构图、对比卡片**；默认用 txt 代码块或 markdown 表格说明。

## 开发规范

**仓库内 `docs/个人开发规范.md` 是本仓库的最高优先级约束，极其重要，不可略读、不可跳过。** 无论代码由人写还是由 AI 写，动代码前**必须完整阅读并逐条对照执行**；写代码、改代码、新增文件前先读它——通用开发规则，条款分红线 / MUST / 取向三级，适用于本仓库全部工程。

## 本机运行环境

**从 Git Bash 调工具、参数里带远端绝对路径时，一律先 `export MSYS_NO_PATHCONV=1`。** MSYS 会把 `/home/u/x` 这类参数改写成 `C:/Program Files/Git/home/u/x`；路径里的空格撞上工具的空格禁令，报出的原因与真因无关（报「路径参数中间不能包含空格」，实际是路径被换了）。转换在 MSYS 运行期发生，shell 引号挡不住它；PowerShell 与 cmd 没有这个问题。

## Agent skills

### Issue tracker

Issues、spec 和 ticket 以仓库内 `docs/issues/<feature-slug>/` 下的 markdown 文件存放。详见 `docs/agents/issue-tracker.md`。

### Triage labels

五个标准分诊角色，标签名与角色名一致（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`）。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文（single-context）：根目录 `CONTEXT.md` + `docs/adr/`。详见 `docs/agents/domain.md`。
