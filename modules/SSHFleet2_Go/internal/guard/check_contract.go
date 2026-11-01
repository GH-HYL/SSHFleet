package guard

import "go/ast"

// CheckContract 是类四：契约包是不是只剩形状。
//
// 判据（ADR-0032 第二条）：契约包里出现函数就报，带接收者的方法也算；类型、常量、
// 包级变量不报（变量只许当哨兵错误这类不可变的值用）。_test.go 不享受豁免——
// 契约里也不该长测试辅助代码。
func CheckContract(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if p.Rule == nil || p.Rule.Kind != KindContract {
			continue
		}
		for _, f := range p.Files {
			for _, decl := range f.AST.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				what := "在契约包里声明了函数 " + fn.Name.Name
				why := "契约里放函数，抽屉就有了第一道门：有第一个就有第二个，纸就会长成行为"
				if fn.Recv != nil {
					what = "在契约包里声明了方法 " + fn.Name.Name
					why = "带接收者的方法也是函数：它把行为挂在纸上，纸从此要跟着逻辑改"
				}
				out = append(out, f.violation(ClassContract, fn.Pos(), what, why,
					"把这段行为放进用到它的那一站；契约只放类型与常量"))
			}
		}
	}
	return out
}
