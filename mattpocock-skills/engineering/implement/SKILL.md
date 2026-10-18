---
name: implement
description: "基于一份规格或一组工单实现一块工作。"
disable-model-invocation: true
---

实现用户以规格或工单形式描述的工作。

如果用户传入了一个工单引用，就从问题追踪器（issue tracker）取回它，并在开始前说明它的标题。如果引用有歧义，就问。

尽可能在事先约定好的接缝（seam）处，调用 Skill 工具并传入 "tdd"。

定期运行类型检查、定期运行单个测试文件，最后完整跑一遍测试套件。

完成后，调用 Skill 工具并传入 "code-review" 来复查这项工作。

把工作提交到当前分支。
