# Learning Record Format（学习记录格式）

学习记录存放在 `./learning-records/`，使用连续编号：`0001-slug.md`、`0002-slug.md`，依此类推。目录按需创建：只在写第一条记录时才建。

它们是教学版的 ADR：记录那些不那么显然的经验教训、关键洞见，以及用户自述的已有知识，这些东西会引导未来的会话。它们用来推算最近发展区。

## Template（模板）

```md
# {Short title of what was learned or established}

{1-3 sentences: what was learned (or what prior knowledge was established), and why it matters for future sessions.}
```

整个格式就是这样。一条学习记录可以只有一段话。价值在于记下**这件事**现在已知，以及它**为什么**改变了接下来该教什么，而不在于把各个小节填满。

## Optional sections（可选小节）

只在它们确实带来价值时才加。大多数记录都不需要。

- **Status** frontmatter（`active | superseded by LR-NNNN`）：当早先的理解被证明有误并被取代时有用。
- **Evidence**：用户是如何表现出这份理解的（答对了一个问题、完成了一个练习、引用了过往经验）。当这个结论日后可能被重看时有用。
- **Implications**：这为未来的会话解锁了什么、排除了什么。不显然时值得记下。

## Numbering（编号）

扫一遍 `./learning-records/`，找出已有的最大编号，然后加一。

## When to write a learning record（什么时候写学习记录）

以下任意一条成立时就写：

1. **用户展现出对某件不那么简单的事的真正理解**：不只是接触过，而是有证据表明他们能正确使用这个概念。这为接下来教什么定下了新的地板。
2. **用户自述了已有知识**：「X 我已经会了。」记下来，免得未来的会话重复教它。同时记下所声称的**深度**。
3. **一个误解被纠正了**：用户先前相信了错误的东西，现在明白了为什么错。这类记录价值很高：它们能预示相关主题上未来的绊脚石。
4. **使命因学习而改变**：用户发现自己真正在乎的，和他们以为的不一样。交叉链接到 [[MISSION.md]] 并更新它。

### What does _not_ qualify（什么不算）

- 仅仅是「讲过了」的材料。讲过不等于学会。等证据。
- 已经在 [[GLOSSARY.md]] 里作为术语定义被简洁记录过的东西。不要重复。
- 逐次会话的活动日志。学习记录不是日记：它们是决策级别的洞见。

## Supersession（取代）

当后来的记录与早先的记录矛盾时（用户的理解加深或纠正了），把旧记录标为 `Status: superseded by LR-NNNN`，而不是删掉它。理解如何演变的历史本身就是有用的信号。
