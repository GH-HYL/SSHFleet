# 派子 agent

**这一篇只管 WorkBuddy 这一个 agent。** 它的后台 subagent 有回执、有落盘、有轮询这几件事；换一个 agent，subagent 未必是这个形状，照搬只会多出一层用不上的步骤。

**能并行的活派后台。** 不带 `run_in_background` 的在前台跑，阻塞本轮——长任务看着像卡死，容易被当成故障打断。超过一两分钟的活一律后台。

调用形状；一条消息里发多个，就是并行起多个：

```json
{"subagent_type": "general-purpose", "description": "三到五字",
 "name": "队内唯一", "run_in_background": true, "prompt": "自包含的完整指令"}
```

**`name` 是唯一的抓手。** 不带它，事后 `SendMessage` 找不到这个 agent；队里重名会找错人。

## prompt 自包含

它看不到本会话，本会话里说过的一律不算。**MUST** 写全：

- 工作目录；
- 任务书在哪：让它读哪份文件，或把要求直接抄进去；
- 文件域白名单，以及不碰什么、那归谁；
- 只读还是可写；可写时，唯一能写的是哪一个文件；
- 授权那句话照红线 1 写；
- 分母与格式要求、回报方式。

## 收

- 跑完回来一条 `<teammate-message teammate_id="…">`，另一条 `teammate_id="system"` 报完成状态与耗时。
- prompt 里要它 `SendMessage`（`recipient: "main"`）的，会回来一条摘要。
- **MUST NOT 轮询。** `TaskOutput` 是快照，`block: true` 对 teammate 也不阻塞——反复查只是白等。

## 落盘

消息会截断、会丢。长结论让它自己写文件，**回执路径写死在 prompt 里**，消息只带摘要。**文件才是持久产物。**

## 中断与停

- 打断本轮＝在跑的后台 agent 被杀，状态变 `killed`，产物可能只写了一半。**先把 prompt 定好再起，起完别在同一轮里改主意。**
- 跑完的自己空闲，不必管；要主动停用 `TaskStop`，**MUST NOT** 用 shell 去 kill 进程。
