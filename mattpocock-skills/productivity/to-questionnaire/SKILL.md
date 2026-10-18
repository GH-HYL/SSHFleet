---
name: to-questionnaire
description: 把一个你无法完全回答的决策，变成一份交给别人填写的问卷。
disable-model-invocation: true
---

把用户独自答不了的东西变成一份**问卷（questionnaire）**：一份 Markdown 文档，交给某一个人异步填写，或者在会上一起填。收件人掌握着用户所缺的知识；问卷就是把这些知识从他那里拉出来。

**拷问「发送」，不要拷问「主题」。** 只就**发送**这件事访谈用户，这是他们总能回答的：发给谁，以及需要拿回什么。文档里的问题则瞄准收件人所知道的与用户所需要的之间的那个**缺口（gap）**。


1. **这是发给谁的？** 用一次问答问清收件人的角色、专长，以及和用户的关系。这决定了问卷的语气，以及它必须自带多少上下文。当你知道收件人是谁、以及他们知道哪些用户不知道的东西时，就算完成。

2. **你需要拿回什么？** 用一次问答问清用户独自解决不了、需要向这个人要的那些具体决策或事实。当你手上有一份明确的清单，写明用户必须能带着走、能做或能定下来的东西时，就算完成。

3. **写问卷。** 针对第 1–2 步得出的缺口起草问题，遵循下面的 Document structure。把它写到当前目录下的 `to-questionnaire-<slug>.md`（slug 取自主题），并回报路径。当文件已存在，且用户在第二步列出的每一项都被某个问题覆盖时，就算完成。

## Document structure（文档结构）

把文档定位成一份**探询问卷（discovery questionnaire）**：用户缺上下文，收件人有。问题按重要性从高到低排序，因为异步意味着你可能只有一次机会；条数多了之后，用 `##` 标题按主题分组。用下面的模板来写。

<questionnaire-template>

# <Questionnaire title>

**Purpose:** why this questionnaire exists and the decision riding on it.

**From:** <the user>, **To:** <the recipient>, **How your answers will be used:** <where they go>

## Context

One paragraph orienting a recipient who wasn't in the user's head. Enough to answer well, not a page.

## How to answer

Deadline and rough effort. Partial answers and "I don't know" are useful: flag anything you're unsure of rather than skipping it.

## <Theme heading>

One `##` section per theme. Under each, its questions, most-important-first. Every question is one idea, never compound, with an answer stub directly beneath, and a one-line _why this matters_ only where the question could be misread or invite a throwaway answer.

<question-example>
### What load is the system expected to handle at launch?

_Why this matters: it decides whether we provision for burst traffic now or defer it._

>
</question-example>

## Anything else?

A closing catch-all: anything we didn't ask that we should know?

</questionnaire-template>
