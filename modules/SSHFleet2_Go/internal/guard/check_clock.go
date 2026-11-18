package guard

import (
	"fmt"
	"go/ast"
	"slices"
)

// checkClock 是类二第三条窄判据：只有塔尖、执行站与日志包能读当前时刻。
//
// 判的是调用点，不是 import：`archive.Create` 收一个 `time.Time`、`log` 收一个时刻来格式化，
// 都是合法的——可疑的是「谁来定现在几点」，那是一处必须在场的事实，随手取一次，那条路径
// 在测试里就再也对不准（开发者 2026-11-02：执行站要实时，建目录那一处也要，其余不许）。
func checkClock(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if clockHome(p) {
			continue
		}
		for _, f := range p.Files {
			if !scans(ClassDeps, f) {
				continue
			}
			ast.Inspect(f.AST, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok || f.ImportPath(id.Name) != "time" || !slices.Contains(clockReaders, sel.Sel.Name) {
					return true
				}
				out = append(out, f.violation(ClassDeps, sel.Pos(),
					fmt.Sprintf("%s 读了当前时刻：time.%s", p.Short, sel.Sel.Name),
					"「现在几点」只在三处取：塔尖建归档目录、执行站算耗时与超时、日志包打时间戳。别处随手取一次，那条路径在测试里就对不准了",
					"要时间就让它从参数进来（收一个 time.Time 往下传），取时刻留在上面那三处"))
				return true
			})
		}
	}
	return out
}

// clockHome 判这个包是不是许读时钟的那三个之一。
//
// 塔尖按规则表那一行的类别认，不按短名——它在表里写的是模块名 `sshfleet2`，不叫 main。
func clockHome(p *Package) bool {
	if p.Rule != nil && p.Rule.Kind == KindTip {
		return true
	}
	return slices.Contains(clockHomes, p.Short)
}
