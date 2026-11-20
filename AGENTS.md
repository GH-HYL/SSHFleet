# AGENTS.md

**这一页是入口，不装规则。** 规则平铺在 `docs/agents/` 下，分两族：`必读-` 每轮开工读，`按需-` 照动作查。这里只写一件事——什么时候读哪一份。

## 必读

| 时机 | 读 |
| --- | --- |
| 动手之前 | `docs/agents/必读-总则.md`、`docs/agents/必读-约束.md` |

## 按需

| 触发场景 | 读 |
| --- | --- |
| 动手写代码、选落点、写测试 | `docs/agents/按需-写代码.md` |
| 每次跑测试（`tools/test.sh`）、连目标机 | `docs/agents/按需-测试环境.md` |
| 起规格、拆工单、写或改 ADR／spec／工单 | `docs/agents/按需-写文档.md` |
| 报「做完了」、提交、写 CHANGELOG、碰版本号 | `docs/agents/按需-交付与提交.md` |
| 写报错、提示、帮助、配置注释、手册 | `docs/agents/按需-给使用者看的文字.md` |
| 听到口述的触发词 | `docs/agents/按需-触发词表.md` |

## Agent skills

下面三样是常驻约定，不进按需表；要改它们，改各自指向的文件。

### Issue tracker

**议题**一个功能一个目录，`docs/issues/<yyyy-MM-dd-HHmm-主题>/`，内放 `spec.md`；拆到工单时再加 `issues/NN-*.md`。见 `docs/agents/issue-tracker.md`。

### Triage labels

六个分诊角色，标签名与角色名一致（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`、`done`）。见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文（single-context）：根目录 `GLOSSARY.md` + `docs/adr/`。见 `docs/agents/domain.md`。
