# 02: 统计与呈现侧改用名字表（删 ModeOf、签名换 cli.Mode）

Status: resolved
Blocked by: 01

**要做的**：把统计与呈现侧的叫法与签名收敛到名字表。本票与 03 改的文件不重叠，可并行；两票同属**一次**改动，一起验收。

**涉及**：

- `result/statistics.go`：删 `ModeOf`；`Statistics` 改取 `a.ModeName()`；成功分类改读 `Info().SuccessCategory`；「是不是传输」改读 `Info().Transfer`（**顺手消掉**「不是执行就算传输」与「是上传/下载才算传输」两套判法——8.1.1 已对齐过一处，本票把两处并成查表）。
- `output/terminal.go`：`ActionName`、`displayCategory` 入参换 `cli.Mode`，取值改查表。
- `output/reporter.go`：`Reporter.mode` 字段与 `NewReporter` 入参换 `cli.Mode`；三处 mode 用法——终端明细开关（`execute || passwd`）、`logNode`、`successDetail`——改成查表或按枚举分支。
- `output/output.go`：`Render` 里 `mode := result.ModeOf(a)` 改 `a.ModeName()`；两处 xlsx 调用随之。
- `output/xlsx.go`：`ActionName` / `displayCategory` 两处调用跟改类型。
- `output/progress.go`：进度模型的 mode 字段与 `newProgressModel` 入参换 `cli.Mode`；传输判定改查表。
- `main.go`：`mode := result.ModeOf(args)` 改 `args.ModeName()`。
- 测试调用点机械同步（`output/*_test.go` 里传 `"execute"` / `"upload"` 的地方改传 `cli.ModeCommand` / `cli.ModeUpload`）。

- [x] `result.ModeOf` 已删除，全仓无引用
- [x] 除 `cli` 的名字表与 `ModeName()` 外，**包外不再出现模式字符串字面量**（`"command"` / `"script"` / `"upload"` / `"download"` / `"passwd"`）
- [x] 「算不算传输类」全仓只剩 `Info().Transfer` 一处判据
- [x] 五个模式的终端明细、统计成功分类、进度界面显示与改动前一致
- [x] `go vet ./...` 干净、`go test ./...` 全绿

## Comments
- 2026-10-07 完成，提交 `b2956d5`。`result.ModeOf` 与三个 `SuccessCategory*` 常量已删；「算不算传输类」现在全仓只有 `Info().Transfer` 一处。
- 两处顺带改动，走查时请留意：终端明细上屏判定由 `mode=="execute"||mode=="passwd"` 改为 `!Info().Transfer`（等价）；`successDetail` 的改密分支并入 `default`（`ActionName` 给「改密」，输出逐字不变）。
- `verdict_test.go` 的 `judgeCorpus` **保留**字面量期望（不读名字表），只订正了它那句已过时的注释——期望值不该跟着实现走，实现改错了测试才会响。
