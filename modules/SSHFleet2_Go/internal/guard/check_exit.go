package guard

import "go/ast"

// CheckExit 是类三：谁自己让进程退场。
//
// 判据（ADR-0032 第五条、ADR-0034 决定四）：os.Exit 的调用点只许在塔尖；「打日志再退」那种
// 收口也归这一处。别处只许把结论往上交——各站拿不到退出这个东西，才是「想停停不了」的结构。
func CheckExit(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if p.Short == m.ModulePath {
			continue
		}
		for _, f := range p.Files {
			ast.Inspect(f.AST, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok || f.ImportPath(id.Name) != "os" || sel.Sel.Name != "Exit" {
					return true
				}
				out = append(out, f.violation(ClassExit, sel.Pos(),
					"在 "+p.Name+" 包里调了 os.Exit",
					"退出是动作，只有塔尖能做；这一站自己退，收尾、日志、退出码都没人接管",
					"把结论（放行／拒绝／起不来）交给塔尖，由它换退出码再退"))
				return true
			})
		}
	}
	return out
}
