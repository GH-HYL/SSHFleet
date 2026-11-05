package guard

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"
)

// consolePkg 是唯一许往终端写字的包。
const consolePkg = "console"

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

// CheckDeps 是类二：谁 import 谁。
//
// 三档（ADR-0032 第四条）：生产文件按规则表那一行；普通 _test.go 豁免；红线测试不享这条豁免，
// 按它自己的 import 白名单判（那一条在类六）。另含三条窄判据：塔尖的点名符号、模块之外的
// import、谁许读当前时刻（checkClock）。
//
// 规则表与真实包清单对不上也归这一类：「多一个」「少一个」都是这张尺子失准，不另立类号
// （map.md 阶段表 01 那一行）。
func CheckDeps(m *Module) []Violation {
	var out []Violation

	out = append(out, reconcile(m)...)

	for _, p := range m.Packages {
		for _, f := range p.Files {
			if !scans(ClassDeps, f) {
				continue
			}
			out = append(out, checkImports(m, p, f)...)
		}
	}
	out = append(out, checkTipSymbols(m)...)
	return append(out, checkClock(m)...)
}

// reconcile 是包清单对账：规则表多一行、少一行都报。
func reconcile(m *Module) []Violation {
	var out []Violation

	present := map[string]bool{}
	for _, p := range m.Packages {
		if p.Rule == nil {
			out = append(out, violationAt(ClassDeps, p.Path,
				fmt.Sprintf("包 %s 不在规则表里（规则表十七行，没有它）", p.Path),
				"包清单以规则表为准：多一个包而表里没有，它属于哪一类、能依赖谁就没人说得清",
				"要么删掉这个包，要么先改规则表并说清它属于哪一类——规则表是约束类，改它要开发者点头"))
			continue
		}
		present[p.Rule.Package] = true
	}

	for _, r := range m.Rules {
		if present[r.Package] {
			continue
		}
		out = append(out, violationAt(ClassDeps, m.ModulePath,
			fmt.Sprintf("规则表里的 %s 在工程里不存在", r.Package),
			"表里少一个包，等于有一条边界没有对应的代码：要么包被悄悄删了，要么路径改了而表没跟上",
			"把包补回来；确要少一个包，先改规则表并说清为什么"))
	}
	return out
}

// checkImports 按规则表那一行判一个生产文件的 import。
func checkImports(m *Module, p *Package, f *SourceFile) []Violation {
	var out []Violation
	for _, local := range sortedImportNames(f) {
		path := f.Imports[local]
		what, ok := m.Internal(path)
		if !ok {
			if IsStdlib(path) {
				continue
			}
			if namedImport(path) {
				continue
			}
			out = append(out, f.violation(ClassDeps, f.importPos[path],
				fmt.Sprintf("import 了模块之外的 %s，它不在点名白名单里", path),
				"模块之外的依赖要点名：白名单外的第三方包一律报红，供应链要一条条认得出来，不是「第三方随便用」",
				fmt.Sprintf("要的包先在标准库里找；确实不够，在 guard 的规则表里加一个名并说清用途（加名记一行，新增一类用途要开发者点头）。今天点名的是：%v", thirdParty),
			))
			continue
		}
		if p.Rule == nil {
			continue // 「多一个包」上面已经报过，不重复
		}
		if slices.Contains(p.Rule.Deps, what) {
			continue
		}
		out = append(out, f.violation(ClassDeps, f.importPos[path],
			fmt.Sprintf("%s 依赖了 %s", p.Short, what),
			whyNotAllowed(p.Rule, what),
			fmt.Sprintf("%s 只许依赖 %s", p.Short, depList(p.Rule.Deps)),
		))
	}
	return out
}

func whyNotAllowed(r *Rule, what string) string {
	switch r.Kind {
	case KindStation:
		return fmt.Sprintf("站之间单向：一站只许依赖契约与九个横切包，不许依赖别的站（依赖 %s 就是往回走）", what)
	case KindCrosscut:
		if r.Package == "ask" {
			return "横切包是叶子：只许依赖标准库与契约，叶子之间的例外只有 ask → console、ask → message 两条"
		}
		return "横切包是叶子：只许依赖标准库与契约，反向依赖站就成环了"
	case KindContract:
		return "契约不属于任何一站：上游填它、下游读它，它自己不依赖工程里的任何包，否则谁想用这张纸就得连那一站一起拉进来"
	case KindKeyTool:
		return "密钥管理不进流水线：只许依赖标准库、契约与横切包，站也不许反过来依赖它"
	case KindCheck:
		return "结构检查只读源码：它一旦 import 站或契约，就把被检查的东西变成了自己的依赖"
	case KindTip:
		return "塔尖只许依赖五站、九个横切包、契约与密钥管理包——它的薄厚看依赖清单，不看行数"
	}
	return "这一行不在规则表允许的依赖范围内"
}

func depList(deps []string) string {
	if len(deps) == 0 {
		return "标准库（不依赖工程里任何包）"
	}
	return "标准库与 " + fmt.Sprintf("%v", deps)
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

func sortedImportNames(f *SourceFile) []string {
	names := make([]string, 0, len(f.Imports))
	for name := range f.Imports {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// namedImport 判一条模块之外的 import 在不在点名白名单里。
//
// 按「库」认，不按包认：登记一个名，它的子包一起放行。理由是同一条供应链上的东西——
// 认领 `charmbracelet/bubbles` 就等于认领它下面的 `bubbles/progress`，一行一个子包只会
// 让名单变成一张包清单，看不出「认了哪几个库」。
func namedImport(path string) bool {
	for _, name := range thirdParty {
		if path == name || strings.HasPrefix(path, name+"/") {
			return true
		}
	}
	return false
}
