---
name: domain-modeling
description: 构建并打磨项目的领域模型（domain model）。在讨论代码库术语、撰写或编辑 GLOSSARY.md，或记录、编辑 ADR 时使用。
---

# 领域建模（Domain Modeling）

在设计的过程中主动构建并打磨项目的领域模型。这是*主动*的纪律：质疑术语、编造边界场景，并在词表和决策一旦成型的当下就把它们写下来。（仅仅为了查词汇而*读* `GLOSSARY.md` 不算本技能：那是一行字的习惯，任何技能都能做。本技能适用于你要改动模型、而不只是消费模型的时候。）

## 文件结构

多数仓库只有一个上下文（context）：

```
/
├── GLOSSARY.md
├── docs/
│   └── adr/
│       ├── 0001-event-sourced-orders.md
│       └── 0002-postgres-for-write-model.md
└── src/
```

如果根目录存在 `GLOSSARY-MAP.md`，则该仓库有多个上下文。这张图指向每个上下文所在的位置：

```
/
├── GLOSSARY-MAP.md
├── docs/
│   └── adr/                          ← system-wide decisions
├── src/
│   ├── ordering/
│   │   ├── GLOSSARY.md
│   │   └── docs/adr/                 ← context-specific decisions
│   └── billing/
│       ├── GLOSSARY.md
│       └── docs/adr/
```

懒创建文件：只有在你确实有东西要写时才创建。如果没有 `GLOSSARY.md`，就在第一个术语被确定时创建一个。如果没有 `docs/adr/`，就在需要第一个 ADR 时创建它。

## 会话期间

### 用词表来质疑

当用户使用的术语与 `GLOSSARY.md` 中已有的语言冲突时，立刻指出来。“你的词表把 'cancellation' 定义成 X，但你似乎指的是 Y。到底是哪个？”

### 打磨模糊的语言

当用户使用含糊或超载的术语时，提出一个精确的规范术语。“你说的是 'account'：你指的是 Customer 还是 User？这是两个不同的东西。”

### 讨论具体场景

在讨论领域关系时，用具体场景对它做压力测试。编造一些能探查边界情况的场景，迫使用户把概念之间的界限说清楚。

### 与代码交叉核对

当用户陈述某处是如何工作的，检查代码是否同意。如果发现矛盾，就摆出来：“你的代码取消的是整个 Order，但你刚说部分取消是可能的。哪个是对的？”

### 就地更新 GLOSSARY.md

当一个术语被确定时，当场更新 `GLOSSARY.md`。不要攒着批量做：一发生就记下来。使用 [GLOSSARY-FORMAT.md](./GLOSSARY-FORMAT.md) 中的格式。

`GLOSSARY.md` 应当完全不包含实现细节。不要把 `GLOSSARY.md` 当成规格、草稿纸，或实现决策的存放处。它是词表，仅此而已。

### 谨慎地提议 ADR

只有当以下三条全部成立时，才提议创建一个 ADR：

1. **难以回退**：以后改主意的代价是实在的
2. **脱离上下文会让人意外**：未来的读者会疑惑“他们为什么这么做？”
3. **是真实取舍的结果**：确实存在其他选项，而你是出于具体理由选了这一个

如果三条缺了任何一条，就跳过 ADR。使用 [ADR-FORMAT.md](./ADR-FORMAT.md) 中的格式。
