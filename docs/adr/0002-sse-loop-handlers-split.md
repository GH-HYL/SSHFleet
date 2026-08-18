# ADR-0002 — go_to_go SSE 循环消息处理拆分

- 日期：2026-08-17
- 状态：Accepted

## 背景

`modules/SSHFleet_Py/src/gotogo/go_to_go.py` 的 `go_to_go()` 上一轮已提取 `_build_progress` / `_record_result` / `_shutdown_go`，但主干仍 287 行，其中 **SSE 接收循环约 166 行**（占主干近 6 成），循环内 12 个可变状态与 7 个 UI 引用交织，单函数过长不便阅读。

## 决策

1. **提取 `SseSession` dataclass** 打包一次 SSE 接收循环的共享上下文：上传模式状态（active_bars / node_approximate / node_total_bytes / completed_nodes / upload_success|fail_nodes / total_uploaded / global_total_bytes）、命令模式状态（success_nodes / fail_nodes）、结果与输出（results / output_file）、UI 引用（progress_table / node_progress / node_task / total_progress / total_task / node_bars / speed_tracker / live）。handler 通过 `session` 原地读写。
2. **按消息类型拆 4 个 handler**：`_handle_init` / `_handle_progress` / `_handle_result` / `_handle_done`；每个返回 `bool`（True=继续循环，False=结束循环；progress 排队满用提前 `return True` 表达 continue 语义）。只读执行参数（args / error_keywords / exec_mode / total_nodes）作为 handler 显式参数传入。
3. **删除"兼容旧 result 格式"的 else 分支**，未知消息类型改为 `tlog.warning` 告警后忽略（决策 B）。事实依据：Go 端源码确认 SSE 消息仅 4 种 type（init/progress/result/done），无空 type/未知 type 发送路径；原 else 为死分支，且其"未知消息当结果处理"的宽容逻辑存在脏数据隐患（拼错 type 或未来新类型会被误记为节点结果）。
4. **已知 type 但当前模式不适用的消息**（如命令模式收到 init/progress）同样忽略而非落回结果处理——Go 端命令模式实际只发 result/done，此差异不影响真实执行，且更安全。
5. **拆分范围**：只拆 SSE 循环；请求构建、Go 进程启动、健康检查线程等阶段（各 ~15 行）留在主干。

## 后果

- 主干预计 287 → ~120 行，SSE 循环收缩为分派壳 + 4 个独立 handler，每个消息类型的处理可独立阅读。
- 正常执行路径行为零变化（4 种 type 的处理逻辑原样搬移）。
- 未知/非预期消息不再被误当节点结果，消除脏数据隐患。
