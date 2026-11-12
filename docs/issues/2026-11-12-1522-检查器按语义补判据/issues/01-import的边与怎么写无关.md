# 01: import 的边与怎么写无关

**要做的**：站里用 `import _ "别的站"` 或 `import . "别的站"` 引入一条反向依赖时，跑测试就红。

**Blocked by**：无，可立即开工。

**Status**：done

- [x] 解析 import 时把 `_` 与 `.` 的路径也记下来，判依赖时一并过规则表
- [x] 样本里加两处（空白 import、点 import），判据报且只报这两条
- [x] 真工程整跑仍零违规
- [x] `go test ./internal/guard/` 绿；指纹变化记进 Comments

## Comments

- 2026-11-12：`SourceFile` 多一格 `SideImports`，把 `_` 与 `.` 的路径记下来（原来在 `scan.go` 直接 `continue` 掉了，边就这么丢了）。`checkImports` 拆出一个 `judgeImport`，对有名字的 import 与这一族各过一遍——判据只关心「这条边指向谁」，不关心它在 import 里叫什么。
- 样本：`violations/internal/gate/side.go` 点 import 问句包、`violations/internal/parse/side.go` 空白 import 问句包。两条都是真的反向依赖，只是名字没写。
- 类二从 6 条变 9 条：多出的三条里两条是上面这两个样本，第三条是 `parse/zero.go`（类五的零值样本）自己 import 了闸门——样本文件的依赖边也算边，这是判据该有的行为，不是噪音。
- 真工程现在一条空白／点 import 都没有，改完零违规。
