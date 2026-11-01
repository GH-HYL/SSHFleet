package guard

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
)

// CheckTestSite 是类六之一：红线测试站在哪儿、站给外面露几个名字。
//
// 两条判据都按文件讲（ADR-0037 决定三）：
//
//   - import 白名单：一个红线测试文件只许 import 标准库、契约包，以及被它钉的那一个包。
//     钉 0 个是合法的——站在塔尖进程出入口上的测试起二进制、看产出，不 import 任何站。
//   - 出口白名单：每个站包至多导出「一个函数加一个类型」（入口函数、替身插口的类型）。
//     骨架阶段 0 个也算过；多一个就红。
func CheckTestSite(m *Module) []Violation {
	return append(redlineImports(m), stationExports(m)...)
}

func redlineImports(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		for _, f := range p.Files {
			if !f.IsRedline {
				continue
			}
			var pinned []string
			for _, name := range sortedImportNames(f) {
				path := f.Imports[name]
				short, ok := m.Internal(path)
				if !ok || short == "contract" {
					continue
				}
				pinned = append(pinned, short)
			}
			if len(pinned) <= 1 {
				continue
			}
			slices.Sort(pinned)
			out = append(out, f.violation(ClassTestSite, f.AST.Name.Pos(),
				fmt.Sprintf("这个红线测试钉了 %d 个包：%v", len(pinned), pinned),
				"红线测试只许站三种口子：塔尖的进程出入口、一个站的入口、一个横切包的那一件事。钉第二个包，这条测试就成了第二个 main",
				"一条测试只钉一个包，按包拆文件；纸的类型在契约包里，钉契约不算钉第二个包"))
		}
	}
	return out
}

// named 是一个导出名连它的出处，报错要指到那一行。
type named struct {
	name string
	file *SourceFile
	pos  token.Pos
}

func stationExports(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if p.Rule == nil || p.Rule.Kind != KindStation {
			continue
		}
		funcs, types := exportedNames(p)
		if len(funcs) > 1 {
			extra := funcs[1]
			out = append(out, extra.file.violation(ClassTestSite, extra.pos,
				fmt.Sprintf("%s 导出了 %d 个函数：%v", p.Short, len(funcs), names(funcs)),
				"站只许露一个口子：入口函数加替身插口的类型。多露一个，下游就能绕过入口拿站里的东西，站也就不再是深模块",
				"把多出来的那个收回去；确实要给外面，就塞进纸里或从入口递出去，别加导出名"))
		}
		if len(types) > 1 {
			extra := types[1]
			out = append(out, extra.file.violation(ClassTestSite, extra.pos,
				fmt.Sprintf("%s 导出了 %d 个类型：%v", p.Short, len(types), names(types)),
				"站只许露一个类型（替身插口那一型）。纸的类型在契约包，站不该再有自己的公开类型",
				"把纸搬进契约包，或用不导出的类型"))
		}
	}
	return out
}

// exportedNames 数一个站导出的函数与类型；测试文件不算（它不进生产程序）。
func exportedNames(p *Package) (funcs, types []named) {
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		for _, decl := range f.AST.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && ast.IsExported(d.Name.Name) {
					funcs = append(funcs, named{d.Name.Name, f, d.Name.Pos()})
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ast.IsExported(ts.Name.Name) {
						continue
					}
					types = append(types, named{ts.Name.Name, f, ts.Name.Pos()})
				}
			}
		}
	}
	return funcs, types
}

func names(ns []named) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.name)
	}
	return out
}
