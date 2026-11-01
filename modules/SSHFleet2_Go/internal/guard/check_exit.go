package guard

import (
	"go/ast"
	"slices"
)

// exitFamily 是「打日志再退」那种收口的形状：先写一笔，再让进程走。
// ADR-0032 第四条点名了它——与 os.Exit 一样只许在塔尖。
var exitFamily = []string{"Fatal", "Fatalf", "Fatalln", "Panic", "Panicf", "Panicln"}

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
				if !ok {
					return true
				}
				path := f.ImportPath(id.Name)
				short, internal := m.Internal(path)

				if path == "os" && sel.Sel.Name == "Exit" {
					out = append(out, f.violation(ClassExit, sel.Pos(),
						"在 "+p.Name+" 包里调了 os.Exit",
						"退出是动作，只有塔尖能做；这一站自己退，收尾、日志、退出码都没人接管",
						"把结论（放行／拒绝／起不来）交给塔尖，由它换退出码再退"))
					return true
				}
				if (path == "log" || (internal && short == "log")) && slices.Contains(exitFamily, sel.Sel.Name) {
					out = append(out, f.violation(ClassExit, sel.Pos(),
						"在 "+p.Name+" 包里用 log."+sel.Sel.Name+" 这种「打日志再退」的收口",
						"退出动作只许在塔尖：「写一笔再退」也是退，包在 os.Exit 外面的收口同样归那一处",
						"把结论交给塔尖；要记日志就记日志，退不退由塔尖定"))
				}
				return true
			})
		}
	}
	return out
}
