# 02: 改密成功立自己的档（含「算不算传输类」两判法对齐）

Status: resolved
Blocked by: None

**要做的**：给改密在呈现层立自己的档，统计与每一处明细都不再把它算成「执行」。

**现状**：`result.ModeOf`（`statistics.go:53-62`）只有三档（execute / upload / download），改密走 `default` 落进 `"execute"`；于是统计、终端明细、执行期日志、xlsx 一律把改密当命令，成功时显示「执行成功」「执行: 成功 - Xs」「命令执行成功，退出码 0」。同时 `statistics.go:69` 判「不是执行就算传输」、`terminal.go:84` 判「是上传/下载才算传输」——**两套判法**眼下靠改密落进 execute 这个巧合保持一致，一旦立档就当场分叉（原报告候选四①）。

## 影响面（比报告写的大，实施前必读）

`ModeOf` 的字符串是**呈现层总来源**，立档要逐点补齐它的消费方。给 `ModeOf` 加一路 `case cli.ModePasswd: return "passwd"` 之后，**必须同批处理**：

| 位置 | 现在 | 改后 |
| --- | --- | --- |
| `result/statistics.go:69` | `mode != "execute"` → 传输成功 | 改成「是 upload/download 才算传输」（与展示侧同口径）；并新增改密档 → 新常量 `SuccessCategoryPasswd = "改密成功"` |
| `output/terminal.go:48` `ActionName` | `default` →「执行」 | 加 `case "passwd"` →「改密」 |
| `output/terminal.go:82` `displayCategory` | 非传输即执行成功 | 加改密 →「改密成功」 |
| `output/reporter.go:129` | `r.mode == "execute"` 才打终端明细 | **加 passwd**——漏了这一步，改密明细**不再上屏**（回归） |
| `output/reporter.go:242` `logNode` | `default` →「命令执行成功，退出码 0」 | 加改密行 →「改密成功，耗时 Xs」 |
| `output/reporter.go:288` `successDetail` | `default` →「执行 Xs」 | 加改密 →「改密 Xs」 |

**不需要动的**：`output/progress.go:343/:361`（改密与 execute 同落 else，行为不变）；`output/xlsx.go`（只调 `ActionName` / `displayCategory`，自动跟随新档）。`output/report.go` 的「执行参数」段已按 `ModeName()` 分派，不受影响。

**边界**：只动呈现层与统计口径，**不动结果判定**（`verdict` 只给成败，中文由消费侧按模式给，D2 冻结）。

- [ ] 改密成功的统计块显示「改密成功」，不再是「执行成功」
- [ ] 终端明细行显示「改密: 成功 - Xs」，不再是「执行: 成功」
- [ ] 执行期日志显示「改密成功」（或「改密失败」），不再是「命令执行成功，退出码 0」
- [ ] 改密的终端明细仍然上屏（`reporter.go:129` 未漏加 passwd）
- [ ] 「算不算传输类」只剩一处判法（statistics 与 displayCategory 同口径）
- [ ] 其余四模式的显示一字不变（命令/脚本仍「执行」「执行成功」）
- [ ] `go vet ./...` 干净、`go test ./...` 全绿

## Comments
- 2026-10-07 完成，提交 `83ff09f`。表内 6 个消费点全部改到；`reporter.go:129` 已确认加上了 passwd（改密明细仍上屏）。`progress.go` 无需改（改密与 execute 同落 else，已核）。
- 2026-10-07 真机补记（审计发现本票漏了 spec §四.5 的沙盒实跑记录）：作者事后在同一目标节点（**非 root** 账号）真机跑改密路径，统计块显示「改密成功」、明细出现「改密: 成功」且**仍上屏**（即 `reporter.go:129` 那处没有回归），通过。
- 2026-10-07 审计回填（提交 `1d622fb`）：本票给 `logNode` 加的改密行当初写死了 `"改密成功"` / `"改密失败"`，与名字表的 `SuccessCategory` 重复，也让 ADR-0013 的旁证对不上。已改从名字表取词（`Info().LogName` 组合），**输出文本一字不变**。
