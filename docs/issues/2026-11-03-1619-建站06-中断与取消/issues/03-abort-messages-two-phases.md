# 03: 被中断的机器分成两档，报错原文里各带一句报文

**要做的**：按 Ctrl+C 之后，还没连上的机器归「建连被取消」、已经连上跑在命令中途的归「任务已取消」；两档的报错原文里各有一句工具报文，屏幕与报告不再是 `context canceled` 原文一排分类。

**Blocked by**：01（要能取消）。

**Status**：ready-for-agent

- [ ] 建连阶段被中断：报错原文是「原文在前、`sshfleet.abort.connect_canceled: interrupted while connecting` 在后」
- [ ] 命令与脚本通道跑到一半被中断：原文后接 `sshfleet.abort.canceled: interrupted after session established`
- [ ] 分类仍由判据表认领（判定层不按结构字段起名）；喂固定原文的两条用例，期望分类名写死为「建连被取消」「任务已取消」
- [ ] 顺带复核 05 写下的那四条（代填、改密、上传、下载）用的是同一句报文、同一个次序
- [ ] 真机：三台、`-n 1`、命令 `sleep 30`，跑到一半发 `SIGINT` → 已开跑那台归「任务已取消」，其余进缺额；再用一台连不上的地址触发建连中断 → 归「建连被取消」
- [ ] `gofmt -l .` 空、四项检查与 `go test -race` 绿

**不在这单**：传输与改密通道自身的收尾行为（04、05）。

## Comments

**2026-11-05（已授权开工，代码就位）**

- 报文收口成一个函数 `abortMessage(connected bool)`：两档各一句，五条通道共用。
- 补上缺的两处：建连失败且当时已被取消 → `connect_canceled`；命令与脚本跑到一半被喊停（`endFacts.cancelled`）→ `canceled`。
- 顺带复核 05 那四条（代填、改密、上传、下载）：改用同一处 `abortMessage(true)`，次序仍是「原文在前、报文在后」，字一个没变。
- 验：`TestAbortMessage` 两条逐字（期望抄老版 `shape_test.go` 的字面量）、`TestCategoryOfAbort` 两条分类（喂固定原文、名字写死）绿。
- **未验**：真机两条（三台 `sleep 30` 中途按、连不上的地址触发建连中断）。
