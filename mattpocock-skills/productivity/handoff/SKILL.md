---
name: handoff
description: 把当前对话压缩成一份交接文档，供另一个 agent 接手。
argument-hint: "下一次会话将用来做什么？"
disable-model-invocation: true
---

写一份交接文档，总结当前对话，好让一个全新的 agent 能接着干。保存到用户操作系统的临时目录（`$TMPDIR`，没有就用 `/tmp`；Windows 上是 `%TEMP%`）——不要存到当前工作区。

文档里要包含一节「建议使用的技能（suggested skills）」，写明下一个 agent 应该为哪些技能调用 Skill 工具。

不要重复其他产物（规格、计划、ADR、issue、提交、diff）里已经记下的内容。用路径或 URL 引用它们即可。

抹掉任何敏感信息，例如 API key、密码或可识别个人身份的信息。

如果用户传了参数，把它们当作「下一次会话将聚焦什么」的描述，并据此裁剪文档。
