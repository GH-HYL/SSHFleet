# 03: 日志、报告、归档与参数屏改用名字表

Status: resolved
Blocked by: 01

**要做的**：把剩下四处叫法收敛到名字表，并清掉两处过时注释。与 02 不重叠，可并行。

**涉及**：

- `batch/batch.go`：删 `execModeName`，日志那行改读 `Info().LogName`。
- `output/report.go`：「执行参数」段落的五路 switch 改查表；超时行措辞改读 `Info().TimeoutLabel`。
- `output/archive.go`：归档目录名改取 `Info().DirName`；`ModeNone` 时补 `"unknown"` 的**防御兜底保留**（参数合规检查已保证五者必有其一，但防御不撤）。
- `confirm/confirm.go`：参数屏的模式行改读 `Info().ScreenLabel`；超时行措辞改读 `Info().TimeoutLabel`。

**顺手清掉两处过时注释**（都写着「四」，实际已是五种模式）：
- `batch/batch.go` 的 `task` 结构体上方「四种模式共用」；
- `output/archive.go` 的 `archiveDirName` 里「参数合规检查已保证四者必有其一」。

- [x] `batch.execModeName` 已删除，全仓无引用
- [x] 归档目录名五个模式逐一核对，与改动前**逐字相同**（`<时间>_<DirName>[_备注]`）
- [x] 参数屏与报告的「执行模式」行、超时行，五个模式逐一核对与改动前相同
- [x] 执行期日志的「开始执行任务：…模式 X」那行五个模式逐一核对相同
- [x] `batch/batch.go` 与 `output/archive.go` 两处「四…」过时注释已清
- [x] `go vet ./...` 干净、`go test ./...` 全绿

## Comments
- 2026-10-07 完成，提交 `b2956d5`。`batch.execModeName` 已删，日志那行改取 `Info().LogName`。
- 两处「执行模式」行（参数屏 / 报告）改为名字表给值，其下的字段行仍按 `cli.Mode` 分派——那是结构性差异、不是叫法，照 ADR-0013《2026-10-07 实施记录》记的边界。
- 超时行两处（参数屏 / 报告）各自的三路 switch 收成一次 `Info().TimeoutLabel` 取值，输出逐字不变。
- `batch/batch.go`「四种模式共用」与 `output/archive.go`「四者必有其一」两处过时注释已清（后者改成「五者必有其一」）。
