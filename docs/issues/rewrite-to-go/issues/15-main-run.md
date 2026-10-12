# M3-15 main：第 8 步接线 + 中断语义

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 14

## 范围

- `main.go` 第 8 步：`batch.Run(ctx, args, cfg, nodes, logger, render)`；`ctx` 由 `signal.NotifyContext`（SIGINT/SIGTERM）派生
- **中断语义**（对位旧引擎 + 旧 Python 整理阶段）：收到信号 → 取消 context → worker 停止启动新任务 → **已完成的节点结果照常进入统计 / 报告 / 归档**，并打印「已收到中断信号，SSHFleet 停止执行（已完成节点的结果已写入日志/输出文件）」，退出码 0
- 渲染函数注入：从 `internal/output` 取（M5 实现终端渲染；M3 只立接缝，函数体暂为占位）

## 验证

- 中断：执行中按 Ctrl+C → 已完成节点结果仍在，程序正常收尾
- 无中断时全链路行为不变

## Comments

- 2026-09-14 完成。第 8 步接线 `batch.Run(ctx, …)`，ctx 由 `signal.NotifyContext` 派生；中断后打印旧文案并继续整理（退出码 0）。渲染接缝 `output.ProgressRenderer(os.Stdout)` 已注入——M3 只交付最小可用形态（单行就地刷新的完成/成功/失败 + 字节），M5 替换为完整进度条。日志器补 `Close()`，main 退出前收尾。
