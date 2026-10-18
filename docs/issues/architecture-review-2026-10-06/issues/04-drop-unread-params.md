# 04: 删掉没人读的参数与一处重复调用

Status: done
Blocked by: None

**要做的**：纯事实清理（原报告候选六）——有几个函数被要求接收某个参数，函数体内从头到尾一次都没读它。

**现状**（核对方法：逐函数体内数 `cfg.` / `keyword` 出现次数，下列均为 **0**；实测行号以当前代码为准）：

| 位置 | 没人读的参数 | 备注 |
| --- | --- | --- |
| `output/xlsx.go:135` `WriteOutputXlsx` | `cfg *config.Config` | 删后同步调用点 `output/output.go:44` |
| `output/xlsx.go:267` `WriteResultsXlsx` | `cfg *config.Config` | 同步 `output/output.go:47` |
| `output/archive.go:49` `BackupAssets` | `cfg *config.Config` | 同步 `output/output.go:36` |
| `output/archive.go:207` `archiveDirName` | `cfg *config.Config` | 同步 `output/archive.go:34`（`CreateArchive`） |
| `output/reporter.go:83` `NewReporter` | `kw *verdict.Keywords` | `:47` 定义字段、`:89` 赋值，全包再没读过；`:81` 注释已过时，一并清 |
| `output/archive.go` `CreateLatestHistoryLink` | `cfg *config.Config` | **原报告未列**——本函数本就从头到尾没读过 `cfg` |
| `output/archive.go` `CreateArchive` | `cfg *config.Config` | **原报告未列**——它只把 `cfg` 转给 `archiveDirName`；后者那格删掉后，它自己也变没人读 |

合计 **7 个参数（6 个 `cfg` + 1 个 `kw`）**。后两行是实施时按「同类一并清、否则等于把毛病挪一层」补的，属**有记录的扩界**（见 Comments）。

顺带一处零风险的重复：`output/reporter.go:121` 与 `:132` 用**完全相同的实参**调了两次 `displayCategory(res, r.mode)`——收成一个局部变量复用。

**边界**：只删参数与那次重复调用。**不碰**「渲染函数的参数表整体收窄」（上轮已冻结）。不改变任何行为。

**注意**：删参数会牵动测试调用点（`output/xlsx_test.go`、`output/archive_test.go`、`output/reporter_test.go`），机械同步即可——测试不是本票的目的，但改完必须能编译、能跑。

- [ ] 七个参数删除，生产调用点全部同步（`output/output.go:31/:36/:44/:47`、`main.go:388`）
- [ ] `reporter.go` 的 `kw` 字段与过时注释一并清掉
- [ ] `displayCategory` 每结果只算一次
- [ ] 测试调用点同步，全仓编译通过
- [ ] 产物（终端 / report.txt / 两张 xlsx）逐字不变
- [ ] `go vet ./...` 干净、`go test ./...` 全绿

## Comments
- 2026-10-07 完成，提交 `83ff09f`。**报告漏了两个**：`CreateLatestHistoryLink` 本就无人读 `cfg`、`CreateArchive` 在删掉 `archiveDirName` 的 `cfg` 后变无人读——同类一并清掉，共 6 个 `cfg` + 1 个 `kw`。`reporter.go` 的 `displayCategory` 已收敛为一个局部变量。
