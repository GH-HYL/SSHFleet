# 领域文档

规定各工程技能在探索本仓库代码时，应如何消费本仓库的领域文档。

## 探索之前，先读这些

- 仓库根目录的 **`CONTEXT.md`**；或
- 仓库根目录的 **`CONTEXT-MAP.md`**（若存在）：它指向每个上下文各自的 `CONTEXT.md`，读其中与当前主题相关的那些。
- **`docs/adr/`**：读与即将动手的区域相关的 ADR。多上下文仓库还要查 `modules/<项目名_语言>/docs/adr/` 中该上下文范围内的决策。

若这些文件不存在，**静默继续**。不要指出它们缺失，也不要主动建议预先创建。`/domain-modeling` 技能（经 `/grill-with-docs` 与 `/improve-codebase-architecture` 进入）会在术语或决策真正被确定时再按需创建它们。

## 文件结构

单上下文仓库（绝大多数仓库）：

```
/
├── CONTEXT.md
├── docs/adr/
│   ├── 0001-event-sourced-orders.md
│   └── 0002-postgres-for-write-model.md
└── modules/
```

多上下文仓库（根目录存在 `CONTEXT-MAP.md`）：

```
/
├── CONTEXT-MAP.md
├── docs/adr/                          ← 系统级决策
└── modules/
    ├── ordering/
    │   ├── CONTEXT.md
    │   └── docs/adr/                  ← 上下文内决策
    └── billing/
        ├── CONTEXT.md
        └── docs/adr/
```

## 使用词表的词汇

当你的产出要命名某个领域概念时（issue 标题、重构提案、假设、测试名），使用 `CONTEXT.md` 中定义的术语。不要漂移到词表明确回避的同义词。

如果需要的概念尚未进入词表，这是一个信号：要么你在发明项目并不使用的说法（重新考虑），要么确实存在缺口（记下来交给 `/domain-modeling`）。

## 挑明 ADR 冲突

如果你的产出与已有 ADR 相抵触，显式指出，不要静默覆盖：

> _与 ADR-0007（事件溯源订单）冲突，但值得重开讨论，因为……_
