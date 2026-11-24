# 派子 agent

两边不是同一个形状：先认自己在哪一边，再照那一边办；共通的四条在最后。

## WorkBuddy

**能并行的活派后台。** 不带 `run_in_background` 的在前台跑，阻塞本轮——长任务看着像卡死，容易被当成故障打断。超过一两分钟的活一律后台。

调用形状；一条消息里发多个，就是并行起多个：

```json
{"subagent_type": "general-purpose", "description": "三到五字",
 "name": "队内唯一", "run_in_background": true, "prompt": "自包含的完整指令"}
```

**`name` 是唯一的抓手。** 不带它，事后 `SendMessage` 找不到这个 agent；队里重名会找错人。

- **收**：跑完回来一条 `<teammate-message teammate_id="…">`，另一条 `teammate_id="system"` 报完成状态与耗时。prompt 里要它 `SendMessage`（`recipient: "main"`）的，会回来一条摘要。
- **停**：打断本轮＝在跑的后台 agent 被杀，状态变 `killed`，产物可能只写了一半。要主动停用 `TaskStop`，**MUST NOT** 用 shell 去 kill 进程。

## 豆包工作

**塔是三层，不跳层**：开发者只跟主 agent 说话，主 agent 只能起 OrganizerAgent，它再往下拆活。没有「随手起一个 general-purpose」这条路。

- **起**：`create_agent`，`agent_type` 只认 `OrganizerAgent`；`title` 十六字以内，是给轨迹与界面看的标题，不是任务描述。派出去就往下走、**不等结果**，完成通知会自己回来。同一个请求只派一个；要补东西用 `send_message` 发过去。
- **喂**：原查询与附件自动传下去，附件也到下游子 agent。`hint` 只到 organizer 这一层、**不跨层**——再往下的子 agent 要用的口径，由 organizer 自己摘进它的 `subtask` 或 `hint`。
- **追问同一个活，用 `send_message` 发给那个 organizer**，别另起一个——新起的没有它那份工作区与上下文，看不到它之前的结论。被 `kill_agent` 停掉的那个发不了消息；真要接着做只能重派，并把已有产物的路径交过去。
- **停**：`kill_agent` 只停自己的直接子 agent，停一个连带它下面全部；停掉回不来，但已产出的文件与结论留着。
- **工作区各归各的**：每个 agent 一个工作区，**只在同一个执行环境里互通**；换一台机器就看不到对方的文件。

## 两边共通

- **指令自包含**：它看不到本会话，本会话里说过的一律不算。**MUST** 写全：工作目录、任务书在哪、文件域白名单与不碰什么（那归谁）、只读还是可写（可写时唯一能写的是哪一个文件）、授权那句话照红线 1 写、分母与格式、回报方式。
- **长结论落盘**：消息会截断、会丢。让它自己写文件，回报里写死路径，消息只带摘要。文件才是持久产物。
- **不轮询**：两边都没有「等着」的路——`TaskOutput` 是快照，`block: true` 对 teammate 也不阻塞；`peek_task_progress` 只给滞后进度，不是等待。
- **先定好再起**：起完别在同一轮里改主意；中途改，等于让它按旧指令干完再返工。
