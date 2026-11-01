package guard

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
)

// conclusionField 是同一口径那两棒上结论格必须叫的名字（ADR-0035 决定三、07 号更新记录）。
const conclusionField = "Conclusion"

// PaperPair 是同一口径上相邻两棒的一对纸：上一棒交出去的那张，与本棒往下递的那张。
//
// 本阶段是 0 对——三张纸随阶段 04 落地。落地时补一行：补一行只让检查更严，不改判据。
type PaperPair struct {
	FromPkg  string
	FromType string
	ToPkg    string
	ToType   string
}

// CheckHandoff 是类六之二：交接件是不是比上一棒宽。
//
// 判据（ADR-0035 决定三）：同一口径往下传时，后一张的字段集 = 前一张的字段集 + 一格结论。
// 多一格数据是「这一棒偷拿了不属于它的东西」，少一格是「下游要的东西上游没给」。
// 换口径的两棒不受这条管，所以它只管对账表里写着的那几对。
func CheckHandoff(m *Module) []Violation {
	var out []Violation
	for _, pair := range m.Pairs {
		_, _, from, ok := structOf(m, pair.FromPkg, pair.FromType)
		if !ok {
			out = append(out, missingPaper(m, pair.FromPkg, pair.FromType))
			continue
		}
		toFile, toPos, to, ok := structOf(m, pair.ToPkg, pair.ToType)
		if !ok {
			out = append(out, missingPaper(m, pair.ToPkg, pair.ToType))
			continue
		}

		for _, field := range to {
			if field == conclusionField || slices.Contains(from, field) {
				continue
			}
			out = append(out, toFile.violation(ClassHandoff, toPos,
				fmt.Sprintf("%s 比 %s 多了 %q 这一格", pair.ToType, pair.FromType, field),
				"同一口径往下传时，除结论格外一格都不许加：多一格要么本该在上游给，要么这一棒在替下游做判断",
				"把这一格退回上游 + 一格，或者确认它其实就是结论格"))
		}
		for _, field := range from {
			if slices.Contains(to, field) {
				continue
			}
			out = append(out, toFile.violation(ClassHandoff, toPos,
				fmt.Sprintf("%s 比 %s 少了 %q 这一格", pair.ToType, pair.FromType, field),
				"下游要的东西上游给了，却在这一棒弄丢了：交接件是同一件事的不同棒，不是重新打包",
				"把这一格补进 "+pair.ToType+"，或确认它本就不该在这一口径上"))
		}
		if !slices.Contains(to, conclusionField) {
			out = append(out, toFile.violation(ClassHandoff, toPos,
				fmt.Sprintf("%s 没有叫 %s 的结论格", pair.ToType, conclusionField),
				"每条同一口径的棒都该有自己那一格结论，且名字固定——名字一换，这条判据就再也对不上",
				"把结论格命名为 "+conclusionField))
		}
	}
	return out
}

func missingPaper(m *Module, pkg, typ string) Violation {
	return violationAt(ClassHandoff, m.PkgPath(pkg),
		fmt.Sprintf("对账表里写着 %s.%s，但这个类型不在了", pkg, typ),
		"对账表里的一对纸必须都能找到；少一张，这条判据就没有可比的两边",
		"把类型补回来，或把对账表里这一行删掉——改表要先说清为什么这一棒不再需要比对")
}

// structOf 找一个包里某个结构体类型的字段名集合。
func structOf(m *Module, pkg, typ string) (*SourceFile, token.Pos, []string, bool) {
	p := m.Pkg(pkg)
	if p == nil {
		return nil, token.NoPos, nil, false
	}
	f, ts, ok := TypeSpec(p, typ)
	if !ok {
		return nil, token.NoPos, nil, false
	}
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return nil, token.NoPos, nil, false
	}
	return f, ts.Name.Pos(), fields(st), true
}

func fields(st *ast.StructType) []string {
	var out []string
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			// 内嵌的字段，名字取类型名。
			if n := exprName(f.Type); n != "" {
				out = append(out, n)
			}
			continue
		}
		for _, name := range f.Names {
			out = append(out, name.Name)
		}
	}
	return out
}

// exprName 取一个类型写法的末段名字：T、*T、pkg.T 都取 T。
func exprName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return exprName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}
