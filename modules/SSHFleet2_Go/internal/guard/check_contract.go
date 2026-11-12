package guard

import (
	"go/ast"
	"go/token"
)

// CheckContract 是类四：契约包是不是只剩形状。
//
// 判据（ADR-0032 第二条）：契约包里出现函数就报，带接收者的方法也算；类型、常量、
// 包级变量不报（变量只许当哨兵错误这类不可变的值用）。_test.go 不享受豁免——
// 契约里也不该长测试辅助代码。
//
// 「出现函数」按值判，不按写法判（ADR-0048）：`func F(){}` 与 `var F = func(){}`、
// `var F = 本包里的函数名` 是同一件事，只认前一种等于让绕过成本降到一行赋值。
//
// 类四同时管纸面白名单：契约里裸 `string` 的格子不在 `paper.go` 那张名单上就报
// （ADR-0044，05 号单）。两类同住一处，是因为它们判的是同一件事——纸是不是只剩形状。
func CheckContract(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if p.Rule == nil || p.Rule.Kind != KindContract {
			continue
		}
		funcs := funcNames(p)
		for _, f := range p.Files {
			for _, decl := range f.AST.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					what := "在契约包里声明了函数 " + d.Name.Name
					why := "契约里放函数，抽屉就有了第一道门：有第一个就有第二个，纸就会长成行为"
					if d.Recv != nil {
						what = "在契约包里声明了方法 " + d.Name.Name
						why = "带接收者的方法也是函数：它把行为挂在纸上，纸从此要跟着逻辑改"
					}
					out = append(out, f.violation(ClassContract, d.Pos(), what, why,
						"把这段行为放进用到它的那一站；契约只放类型与常量"))
				case *ast.GenDecl:
					out = append(out, funcValueViolations(f, d, funcs)...)
				}
			}
		}
	}
	return append(out, checkPaperFields(m)...)
}

// funcValueViolations 判一条包级 var 装的是不是函数：函数字面量、或本包里某个函数的名字。
//
// 判的是「值是不是函数」——`var 哨兵 = errors.New(...)` 这类仍放过，那是数据不是行为。
// 跨包取一个函数名（`var F = other.Pkg`）这里判不出来（要看类型信息），不在这一条里。
func funcValueViolations(f *SourceFile, d *ast.GenDecl, funcs map[string]bool) []Violation {
	var out []Violation
	if d.Tok != token.VAR {
		return nil
	}
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, val := range vs.Values {
			name := "?"
			if i < len(vs.Names) {
				name = vs.Names[i].Name
			}
			isFunc := false
			switch v := val.(type) {
			case *ast.FuncLit:
				isFunc = true
			case *ast.Ident:
				isFunc = funcs[v.Name]
			}
			if !isFunc {
				continue
			}
			out = append(out, f.violation(ClassContract, val.Pos(),
				"在契约包里把函数装进了变量 "+name,
				"契约里放函数，抽屉就有了第一道门：写成 `var` 不是形状而是行为，纸从此要跟着逻辑改",
				"把这段行为放进用到它的那一站；契约只放类型与常量"))
		}
	}
	return out
}

// funcNames 是一个包里顶层函数的名字（不带接收者的那些）。
func funcNames(p *Package) map[string]bool {
	out := map[string]bool{}
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		for _, decl := range f.AST.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				out[fn.Name.Name] = true
			}
		}
	}
	return out
}
