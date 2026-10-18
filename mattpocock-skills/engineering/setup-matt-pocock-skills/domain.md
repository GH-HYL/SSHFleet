# 领域文档（Domain Docs）

工程类技能在探查代码库时，应当如何消费本仓库的领域文档。

## 探查之前，先读这些

- 仓库根目录的 **`GLOSSARY.md`**，或者
- 仓库根目录的 **`GLOSSARY-MAP.md`**（如果存在）：它指向每个上下文各一份 `GLOSSARY.md`。读取与当前主题相关的每一份。
- **`docs/adr/`**：读取涉及你即将动手的区域的 ADR。在多上下文仓库中，还要查看 `src/<context>/docs/adr/` 以找到上下文范围内的决策。

如果这些文件中有任何不存在，**静默继续**。不要指出它们缺失，也不要建议提前创建它们。`/domain-modeling` 技能（可经 `/grill-with-docs` 和 `/improve-codebase-architecture` 抵达）会在术语或决策真正被确定下来时惰性创建它们。

## 文件结构

单上下文仓库（大多数仓库）：

```
/
├── GLOSSARY.md
├── docs/adr/
│   ├── 0001-event-sourced-orders.md
│   └── 0002-postgres-for-write-model.md
└── src/
```

多上下文仓库（根目录存在 `GLOSSARY-MAP.md`）：

```
/
├── GLOSSARY-MAP.md
├── docs/adr/                          ← 系统级决策
└── src/
    ├── ordering/
    │   ├── GLOSSARY.md
    │   └── docs/adr/                  ← 上下文专属决策
    └── billing/
        ├── GLOSSARY.md
        └── docs/adr/
```

## 使用词表的词汇

当你的产出要命名某个领域概念时（无论出现在 issue 标题、重构提案、假设还是测试名里），都要使用 `GLOSSARY.md` 中定义的那个术语。不要漂移到词表明确回避的同义词上。

如果你需要的概念还不在词表里，这是一个信号：要么你在发明项目并不使用的说法（重新考虑），要么确实存在一个空白（记下来交给 `/domain-modeling`）。

## 标出 ADR 冲突

如果你的产出与某个已有 ADR 相矛盾，明确地把它摆出来，而不是悄悄推翻：

> _与 ADR-0007（event-sourced orders）相矛盾，但值得重新讨论，因为……_
