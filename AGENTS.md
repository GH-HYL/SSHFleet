# AGENTS.md

## 交付约定

**不要调用文件呈现工具拉起文件。** 产出或改动交付物时，**在回复正文里写出路径即可**，用户会自己按路径去看。

理由：这是用户明确要求的行为，与工具的默认呈现习惯相反；**默认习惯与用户要求冲突时，按用户要求走。**

## 本机运行环境

**从 Git Bash 调工具、参数里带远端绝对路径时，一律先 `export MSYS_NO_PATHCONV=1`。** MSYS 会把 `/home/u/x` 这类参数改写成 `C:/Program Files/Git/home/u/x`；路径里的空格撞上工具的空格禁令，报出的原因与真因无关（报「路径参数中间不能包含空格」，实际是路径被换了）。转换在 MSYS 运行期发生，shell 引号挡不住它；PowerShell 与 cmd 没有这个问题。

## Agent skills

### Issue tracker

Issues、spec 和 ticket 以仓库内 `docs/issues/<feature-slug>/` 下的 markdown 文件存放。详见 `docs/agents/issue-tracker.md`。

### Triage labels

五个标准分诊角色，标签名与角色名一致（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`）。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文（single-context）：根目录 `CONTEXT.md` + `docs/adr/`。详见 `docs/agents/domain.md`。
