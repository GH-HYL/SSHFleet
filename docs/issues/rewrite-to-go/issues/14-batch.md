# M3-14 batch：任务构建 + worker pool + 进度聚合

Type: task
Status: resolved
Resolved: 2026-09-14
Blocked by: 13

## 范围

`internal/batch`（对位旧 `core/runPool` + 三个 executor + `httpserver/batch.go` 的配料表）：

- **任务构建**：按模式造任务（命令 / 脚本 / 上传（本地清单收集）/ 下载）；本地上传清单收集（对位旧 `localfs.CollectFiles`：拒软链接、拒 `.lnk`、拒 FIFO/device/socket、验证可读、目录递归且**只取文件名**、无真实文件报错）
- **worker pool**：`context` 取消感知；`maxConcurrency = clamp(args.Number, 1..任务数)`；任务 channel 提交、结果 channel 收集、`ctx` 取消时静默丢弃；进度回调经 `select` 同时监听 `ctx.Done()`
- **进度聚合器**（跨调用保存状态 → 带方法类型）：汇总各节点字节/文件数/完成数，接收 ssh 的进度回调；与 worker pool 同一生命周期
- **渲染接缝**（spec D2 展开）：`main.go` 把 `internal/output` 的渲染函数作为参数注入（依赖注入，不是回调控制生命周期）；batch 只按节流调用，不决定渲染形态
- 结果：`Results`（各节点 `ssh.Result` 的集合，按 Seq 保序）

## 验证

- 并发数钳制、取消后不再启动新任务、结果条数与节点数一致
- 聚合值：字节累加、完成数、成功/失败文件数

## Comments

- 2026-09-14 完成。真机验证进度聚合：最终快照 completed=1/1、succeeded=1、failed=0、bytes=18/18、节点 Done 且 Bytes=18。上传源与下载落地目录按旧 Python builder 转绝对路径（`filepath.Abs`）。
