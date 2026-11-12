package guard

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
)

// FailureSite 是类五要判的一处：一种失败类型，许在哪些包造、不许在哪些包造。
//
// OnlyIn 非空表示白名单（只许这几个包造）；空表示除 NeverIn 点名的那几个包之外都能造。
type FailureSite struct {
	Pkg     string
	Type    string
	OnlyIn  []string
	NeverIn []string
}

// allows 判某个包造这个类型算不算越界。
func (s FailureSite) allows(short string) bool {
	if len(s.OnlyIn) > 0 {
		return slices.Contains(s.OnlyIn, short)
	}
	return !slices.Contains(s.NeverIn, short)
}

// CheckFailure 是类五：两种失败各许在哪儿造。
//
// 判据（ADR-0032 第四条、ADR-0034 决定三）：「起不来」谁都能造，闸门里造即报；
// 「拒绝」只许闸门造。要防的是「拿起不来当拒绝用」——那是越界最容易的伪装。
//
// 约定的类型不见了也报：第五类没有可判对象时不许静默放过，否则删掉那个类型就能让这条检查闭嘴。
// _test.go 不看：这一条防的是生产路径上把两种失败混用，测试里造一个来断言形状不算越界。
func CheckFailure(m *Module) []Violation {
	var out []Violation
	for _, site := range m.Failures {
		decl := m.Pkg(site.Pkg)
		if decl == nil {
			out = append(out, violationAt(ClassFailure, m.ModulePath,
				fmt.Sprintf("约定的 %s 类型所在的包 %s 不在了", site.Type, site.Pkg),
				"第五类拿这两个类型当判据；包没了，这一条就没有可判对象",
				"把包和类型补回来；确要换落点，先改 ADR-0034 决定三（红线，单独点头）"))
			continue
		}
		if !hasType(decl, site.Type) {
			out = append(out, violationAt(ClassFailure, decl.Path,
				fmt.Sprintf("约定的失败类型 %s.%s 不见了", site.Pkg, site.Type),
				"没有这个类型，第五类就判不出「谁在哪儿造它」——删掉它等于让这条检查闭嘴",
				"把类型补回 "+site.Pkg+"；确要换名字，先改 ADR-0034 决定三（红线，单独点头）"))
			continue
		}
		out = append(out, constructionSites(m, decl, site)...)
	}
	return out
}

func constructionSites(m *Module, decl *Package, site FailureSite) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		for _, f := range p.Files {
			if !scans(ClassFailure, f) {
				continue
			}
			if site.allows(p.Short) {
				continue
			}
			ast.Inspect(f.AST, func(n ast.Node) bool {
				pos, how := constructionAt(n, f, decl, site.Type)
				if pos == token.NoPos {
					return true
				}
				out = append(out, f.violation(ClassFailure, pos,
					fmt.Sprintf("在 %s 包里造了 %s.%s（%s）", p.Short, site.Pkg, site.Type, how),
					reason(site), fixFor(site)))
				return true
			})
		}
	}
	return out
}

// constructionAt 判一个节点是不是「造了这个失败类型」，是就交回位置与写法。
//
// 三种写法都算（ADR-0048）：复合字面量、`new(T)`、一个零值变量。只认第一种，等于让绕过
// 成本降到「换个写法」——而这一条防的是「包外伪造一个拒绝」，写法本来就不该参与判断。
func constructionAt(n ast.Node, f *SourceFile, decl *Package, name string) (token.Pos, string) {
	switch x := n.(type) {
	case *ast.CompositeLit:
		if isTypeOf(x.Type, f, decl, name) {
			return x.Pos(), "复合字面量"
		}
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "new" && len(x.Args) == 1 {
			if isTypeOf(x.Args[0], f, decl, name) {
				return x.Pos(), "new"
			}
		}
	case *ast.ValueSpec:
		if x.Type != nil && len(x.Values) == 0 && isTypeOf(x.Type, f, decl, name) {
			return x.Pos(), "零值变量"
		}
	}
	return token.NoPos, ""
}

func reason(site FailureSite) string {
	if len(site.OnlyIn) > 0 {
		return "拒绝权只有闸门一处：别处能造出「拒绝」，就等于第二个地方能说「不让走」"
	}
	return "「起不来」是前提缺了，不是不放行：闸门里造它，等于把判断的边界搅成一锅"
}

func fixFor(site FailureSite) string {
	if len(site.OnlyIn) > 0 {
		return "把「不让走」的事实交给闸门，由闸门造一个拒绝；别处只交前提齐不齐"
	}
	return "闸门只交「拒绝」（输入不合规、危险命令、当场喊停）；前提缺了归解析站去报"
}

// isTypeOf 判一个复合字面量的类型写法是不是指定的那个类型：同包写 Ident，跨包写 pkg.Type。
func isTypeOf(expr ast.Expr, f *SourceFile, decl *Package, name string) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return f.Pkg == decl && t.Name == name
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if !ok {
			return false
		}
		return f.ImportPath(id.Name) == decl.Path && t.Sel.Name == name
	}
	return false
}

// hasType 判一个包里有没有这个类型声明。
func hasType(p *Package, name string) bool {
	_, _, ok := TypeSpec(p, name)
	return ok
}
