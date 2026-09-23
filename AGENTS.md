# AGENTS.md

## 交付约定

**不要调用文件呈现工具拉起文件。** 产出或改动交付物时，**在回复正文里写出路径即可**，用户会自己按路径去看。

理由：这是用户明确要求的行为，与工具的默认呈现习惯相反；**默认习惯与用户要求冲突时，按用户要求走。**

## Agent skills

### Issue tracker

Issues、spec 和 ticket 以仓库内 `docs/issues/<feature-slug>/` 下的 markdown 文件存放。详见 `docs/agents/issue-tracker.md`。

### Triage labels

五个标准分诊角色，标签名与角色名一致（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`）。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文（single-context）：根目录 `CONTEXT.md` + `docs/adr/`。详见 `docs/agents/domain.md`。
