# WorkBuddy 后台子 agent 怎么起

`Agent` 工具，带 `run_in_background: true`。

```json
{
  "subagent_type": "general-purpose",
  "description": "三到五字",
  "name": "researcher",
  "run_in_background": true,
  "prompt": "自包含的完整指令"
}
```

## 起

- 立刻返回 `agent_id`／`task_id`，本轮不用等。
- 一条消息里发多个 `Agent` 调用＝并行起多个。
- 带 `name` 才能用 `SendMessage` 找到它；同一队里名字不许重。
- **前台（不带 `run_in_background`）会阻塞本轮**，长任务看起来像卡死、容易被当成故障打断。超过一两分钟的活一律后台。

## 收

- 跑完来一条 `<teammate-message teammate_id="…">`；另有 `teammate_id="system"` 的一条报完成状态与耗时。
- prompt 里要求它 `SendMessage`（`recipient: "main"`）时，会回来一条摘要。
- **不要轮询。** `TaskOutput` 只是快照，不阻塞；`block: true` 对 teammate 也一样。

## 让它落盘

消息会截断、会丢；长结论让它自己写文件（回执路径写死在 prompt 里），消息只带摘要。**文件才是持久产物。**

## prompt 必须自包含

它看不到本会话。要写清：

- 工作目录；
- 任务书在哪（让它读哪份文件，或把要求直接抄进去）；
- 文件域白名单，以及「不碰什么、那归谁」；
- 只读，唯一可写的产物是哪一个文件；
- 分母与格式要求、回报方式。

## 中断

打断本轮＝在跑的后台 agent 被杀，状态变 `killed`，产出可能只写了一半。先把 prompt 定好再起，起完别在同一轮里反复改主意。

## 停

跑完的不必管（自己空闲）。要主动停用 `TaskStop`，不要用 shell 去 kill 进程。
