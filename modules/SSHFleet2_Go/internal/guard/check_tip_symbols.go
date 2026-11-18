package guard

import (
	"fmt"
	"go/ast"
	"slices"
)

// tipSymbols 是塔尖能从这三个包里用的符号，点名的几个，不是「这三个包随便用」
// （ADR-0032 第五条）。包路径写在左边，所以 os/signal 与 os 分得开。
// os.Args 是 2026-11-01 补的：argv 是塔尖「把命令行递给解析站」那件事的输入，
// 没有它塔尖连第一格都搬不动；详见 ADR-0032 末尾的更新记录。
var tipSymbols = map[string][]string{
	"os":     {"Exit", "Args"},
	"errors": {"As", "Is"},
	// 这一格留着、但是空的：塔尖不再订阅信号，信号的登记与处置住 interrupt 包（ADR-0042）。
	// 空列表＝一个符号都不许用；把这一格删掉反而会变成「整包放行」，方向正好相反。
	"os/signal": {},
}

// checkTipSymbols 管塔尖对 os／os/signal／errors 的符号级限制。
func checkTipSymbols(m *Module) []Violation {
	tip := m.Pkg(m.ModulePath)
	if tip == nil {
		return nil
	}
	var out []Violation
	for _, f := range tip.Files {
		if !scans(ClassDeps, f) {
			continue
		}
		ast.Inspect(f.AST, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			path := f.ImportPath(id.Name)
			allowed, tracked := tipSymbols[path]
			if !tracked || slices.Contains(allowed, sel.Sel.Name) {
				return true
			}
			out = append(out, f.violation(ClassDeps, sel.Pos(),
				fmt.Sprintf("塔尖用了 %s.%s", id.Name, sel.Sel.Name),
				fmt.Sprintf("塔尖对 %s 只许用点名的几个符号（%v）：它是编排，不是取数据的地方", path, allowed),
				"要用它就在站里用、把结论交给塔尖；确需在塔尖用，先改 ADR-0032 第五条（红线，单独点头）",
			))
			return true
		})
	}
	return out
}
