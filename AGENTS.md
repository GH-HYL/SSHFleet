# AGENTS.md

**规矩都写在 `docs/个人开发规范.md` 里，本文件只放地图。** 粗体都是那份文件里的标题，照它 grep 就能落到行号。

**每次回话之前，先在心里说清核心诉求是什么，再看这一轮的想法和回答是不是服务它；它可以变，但只能明着变。**

## 必读

- **总则**——角色、谁的话算数、条款分几级、什么算废话。（动手之前读）
- **一、约束**——红线，每条自带适用场景；拿不准时怎么问。（动手之前读）
- **五、说话**——怎么回话、说多少；要交结果的那段，按 **7. 报结果** 的四样写。（开口之前读）

只答问题不动手，读 **五、说话** 就够。

## 按需读

- **二、流程**——起不起规格、规格与工单、报「做完了」前验什么、核什么。（起规格、拆工单、报「做完了」时读）
- **三、结构**——目录与落点、代码工程、测试接缝与两区、验证环境。（动结构、选落点、起验证环境时读）
- **四、产出**——提交信息、CHANGELOG、ADR／规格／工单的写法、使用者可见文案。（改代码顺带写 CHANGELOG、提交、写 ADR／手册时读）
- **附：触发词对照表**——触发词与技能的对照。（听到触发词时读：授权开工、拷问、spec、工单、ADR、复查、tdd、缺陷）

> 动作没到不必看，也不必回头自查「我有没有越界」。

## Agent skills

主干流程与载体以规范「二、流程」为准，不另建一套：规格与工单在 `docs/issues/<slug>/`，决策在 `docs/adr/`，词表在 `GLOSSARY.md`。

### Document formats

写 ADR / spec / 工单时**照着内容规范办**：`docs/agents/doc-guide.md`（什么时候写、模板字段、竖切与宽重构怎么拆）。**不要靠翻旧文件猜格式。**

### Issue tracker

**议题**一个功能一个目录，`docs/issues/<yyyy-MM-dd-HHmm-主题>/`，内放 `spec.md` 与 `issues/NN-*.md`；详见 `docs/agents/issue-tracker.md`。

### Triage labels

六个分诊角色，标签名与角色名一致（`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`、`done`）。详见 `docs/agents/triage-labels.md`。

### Skills index

**哪个环节用哪个技能，见 `docs/个人开发规范.md`「附：触发词对照表」**——表里只收执行者自己该调的；审计、交接这类由开发者自己敲，调用一律写斜杠名（如 `/my-grill`），客户端按名字自动索引。

### Domain docs

单上下文（single-context）：根目录 `GLOSSARY.md` + `docs/adr/`。详见 `docs/agents/domain.md`。
