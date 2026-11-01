# 08: 旧 ADR 里最后 41 处 `internal/*` 路径——一条条判，不是机械替换

**要做的**：15 篇旧 ADR 里共 41 行引用了老工程的 `internal/*` 路径（ADR-0032《影响》第一条算的账），建包时漏了换。本单补上：按迁移地图一条条判它现在住哪个包，然后在**各自文末追加更新记录**。

**为什么不就地改**：旧文是当时的判断，改掉就看不出决策怎么变的（`docs/个人开发规范.md` 四.3）。

**为什么不能机械替换**：`internal/cli` 劈进了 `parse` 与 `gate` 两个包——`args.go`／`source.go`／`answer.go`／`interpreter.go`／`script.go`／`remark_*.go` 归 `parse`，`check.go`／`passwd.go` 归 `gate`。哪一句在说哪个文件，得回去读上下文再判。

**账**（现数，按行）：`0021` 11、`0003` 5、`0004` 4、`0014` 3、`0025` 3、`0028` 3、`0012` 2、`0019` 2、`0020` 2，`0005`／`0015`／`0017`／`0022`／`0024`／`0026` 各 1。

**Blocked by**：无，可立即开工（纯文档活，与检查器无关）。

**Status**：done

## 验收

- [x] 15 篇各有一段 `## 更新记录` 条目，写明：日期、那条路径现在住哪个包、上文哪一处不再约束、替代处指到迁移地图
- [x] 全仓 `docs/adr/` 下 `internal/` 只剩 **1** 行——ADR-0032 自己那一处（它说的是「15 篇旧 ADR 里共 41 处」，是账本身，不该换）
- [x] 旧文正文一个字没改：`git diff` 里每篇只多出末尾那几行
- [x] 对照表（老路径 → 新包，一行一条）贴进阶段议题的 `## Comments`
- [x] 老工程目录零改动

## 不在这单

其它文档（手册、议题、`GLOSSARY.md`）里的路径引用——不在 ADR-0032 那份账里，撞见了只报告。

## Comments

### 2026-11-01：交付

41 处换完，`docs/adr/` 下只剩 ADR-0032 自己那一处（它说的是这笔账，不该换）。15 篇各在 `## 更新记录` 末尾追加一条，写明各自的新落点。

**一处判读**：这批路径不是「旧决策被取代」，是引用失效——旧文里的决定、理由、备选一字未动，只把指错的位置换成现在的位置，末尾再留一行说明。这与「旧文不就地改写」的用意一致：看决策怎么变的凭据在，看地址的读者也不再被误导。

**对照表**（老位置 → 新落点）：

| 老位置 | 新落点 | 落在哪几篇 |
| --- | --- | --- |
| `cli/check.go`、`cli/passwd.go`、`dangercheck/check.go`、`confirm/confirm.go` | `gate` | 0003 0014 0019 0025 |
| `cli/args.go`、`cli/source.go`、`cli/answer.go`、`cli/interpreter.go`、`cli/script.go`、`nodelist/*`、`common/csvtext.go` | `parse` | 0014 0020 0025 |
| `batch/*`、`ssh/*`、`output/terminal.go`（结果行那半）、`output/progress.go`、`output/reporter.go`、`result/classify.go`、`verdict/verdict.go` | `execute` | 0003 0004 0005 0012 0015 0021 0022 0025 0026 |
| `result/statistics.go` | `settle` | 0003 |
| `output/output.go`、`output/report.go`、`output/xlsx.go`、`output/terminal.go`（统计块那半） | `report` | 0003 0004 0021 |
| `output/archive.go`、`output/link_*.go` | `archive` | 0003 0017 |
| `common/console_*.go` | `console` | 0024 |
| `log/log.go` | `log` | 0028 |
| `verdict/keywords.go`（读表那半） | `parse` | 0004 0021 |

`internal/cli` 与 `internal/output` 都不止去一处，所以逐条按「这句话在说哪个文件」判，没有机械替换。
