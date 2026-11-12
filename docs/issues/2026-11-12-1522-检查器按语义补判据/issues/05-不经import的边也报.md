# 05: 不经 import 的边也报

**要做的**：工程里出现 `//go:linkname` 指令、或出现任何 `.s` 汇编文件时，跑测试就红——这两样能在 import 表上不留痕迹地建一条跨包边。

**Blocked by**：无，可立即开工。

**Status**：done

- [x] 判据扫两样：源码里的 `//go:linkname` 指令、模块里的 `.s` 文件
- [x] 样本里两样各一处，判据报且只报这两条
- [x] 真工程反向验证：造一份合法汇编，真工程报且只报这一条；撤销后绿
- [x] 真工程整跑零违规（现在 `go:linkname` 零处、`.s` 零个）
- [x] `go test ./internal/guard/` 绿；指纹记进 Comments

## Comments

- 2026-11-12：开发者选了「禁掉这两样」（`docs/adr/0049-不经import的边一律禁.md`），否掉了「读链接产物」那条路——那个要把编译接进检查器，产物里还混着编译器自己的符号，误报难清。
- 判据并进**类二**（依赖方向），不新开类：它说的就是「这条边不许有」，只是这条边不写在 import 里。
- 为此改了两处底子：`Load` 走目录时把 `.s` 记进 `Module.AsmFiles`（原来非 `.go` 文件直接跳过）；解析文件时加 `parser.ParseComments`——`//go:linkname` 是编译指令，只住在注释里。**别的判据仍然只看语法树**，「注释里提到 os.Exit 不算越界」那条不变（ADR-0037 决定二），这一点写在 `parseFile` 的注释里。
- 样本：`testdata/linkedge/` 假模块，`internal/parse/run.go` 一处 linkname、`internal/parse/hop.s` 一份汇编。测试 `TestCheckLinkEdges`。
- 真工程反向验证：往 `internal/parse/` 放一份能汇编过的 `.s`，`TestProjectIsClean` 报且只报一条（`internal/parse/tmp_hop.s [类二 依赖方向] 工程里有汇编文件…`）；删掉后重跑绿。linkname 那一支在真工程里不好造（写成真的会被链接器带走），只在样本里验。
- 指纹：红线名单新增一行 `{"TestCheckLinkEdges", []string{"CheckDeps", "ClassDeps", "Report"}, 5}`，红线 45 → 46。既有条目只改了上一轮那四条。
