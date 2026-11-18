# 02: 依赖检查一文件一条

**要做的**：把 `internal/guard/check_deps.go` 里那五条判据拆成五个文件，与既有八个 `check_*.go` 同形；入口 `CheckDeps` 单独留一个文件。行为一条不改。

**Blocked by**：无，可立即开工。

**Status**：done（done）

- [x] `check_imports.go`——import 边、模块外白名单、按类别的措辞与依赖清单
- [x] `check_reconcile.go`——包清单对账
- [x] `check_tip_symbols.go`——塔尖的点名符号
- [x] `check_clock.go`——谁许读当前时刻
- [x] `check_link_edges.go`——`//go:linkname` 与 `.s`
- [x] `check_deps.go` 只剩入口与它的说明，列清哪条判据在哪个文件
- [x] 两个检查共用的 `consolePkg` 挪到 `check_terminal.go`，`go vet` 无「未使用」
- [x] `gofmt -l internal/` 空、`go vet ./...` 干净、`go test -count=1 ./...` 全绿

## Comments

- 2026-11-18：**已落地**。按判据切、不按行数切：每个文件连同它自己的说明与常量走（`tipSymbols` 跟着 `check_tip_symbols.go`，`linkDirective` 跟着 `check_link_edges.go`）。`sortedImportNames` 留在 `check_imports.go`，`check_testsite.go` 继续跨文件用它——这条旁路原本就有，本轮没动。
- 验收：`bash tools/test.sh ./internal/guard/` 绿；全量另跑一次绿。
