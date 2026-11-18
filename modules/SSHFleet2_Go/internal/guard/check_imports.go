package guard

import (
	"fmt"
	"slices"
	"strings"
)

// checkImports 按规则表那一行判一个生产文件的 import。
func checkImports(m *Module, p *Package, f *SourceFile) []Violation {
	var out []Violation
	for _, local := range sortedImportNames(f) {
		out = append(out, judgeImport(m, p, f, f.Imports[local])...)
	}
	// `_` 与 `.` 两种写法没有名字可引用，但依赖边一样成立：换一种写法绕开规则表，
	// 绕过去的是「这条边不许有」，不是「这个名字不许用」（ADR-0048）。
	for _, path := range f.SideImports {
		out = append(out, judgeImport(m, p, f, path)...)
	}
	return out
}

// judgeImport 判一条依赖边（不管它在 import 里写成什么名字）。
func judgeImport(m *Module, p *Package, f *SourceFile, path string) []Violation {
	var out []Violation
	what, ok := m.Internal(path)
	if !ok {
		if IsStdlib(path) {
			return nil
		}
		if namedImport(path) {
			return nil
		}
		return append(out, f.violation(ClassDeps, f.importPos[path],
			fmt.Sprintf("import 了模块之外的 %s，它不在点名白名单里", path),
			"模块之外的依赖要点名：白名单外的第三方包一律报红，供应链要一条条认得出来，不是「第三方随便用」",
			fmt.Sprintf("要的包先在标准库里找；确实不够，在 guard 的规则表里加一个名并说清用途（加名记一行，新增一类用途要开发者点头）。今天点名的是：%v", thirdParty),
		))
	}
	if p.Rule == nil {
		return nil // 「多一个包」上面已经报过，不重复
	}
	if slices.Contains(p.Rule.Deps, what) {
		return nil
	}
	return append(out, f.violation(ClassDeps, f.importPos[path],
		fmt.Sprintf("%s 依赖了 %s", p.Short, what),
		whyNotAllowed(p.Rule, what),
		fmt.Sprintf("%s 只许依赖 %s", p.Short, depList(p.Rule.Deps)),
	))
}

func whyNotAllowed(r *Rule, what string) string {
	switch r.Kind {
	case KindStation:
		return fmt.Sprintf("站之间单向：一站只许依赖契约与横切包，不许依赖别的站（依赖 %s 就是往回走）", what)
	case KindCrosscut:
		if r.Package == "ask" {
			return "横切包是叶子：只许依赖标准库、契约与判据包（criterion），叶子之间的例外只有 ask → console、ask → message、console → message 三条"
		}
		return "横切包是叶子：只许依赖标准库与契约，反向依赖站就成环了"
	case KindContract:
		return "契约不属于任何一站：上游填它、下游读它，它自己不依赖工程里的任何包，否则谁想用这张纸就得连那一站一起拉进来"
	case KindKeyTool:
		return "密钥管理不进流水线：只许依赖标准库、契约与横切包，站也不许反过来依赖它"
	case KindCheck:
		return "结构检查只读源码：它一旦 import 站或契约，就把被检查的东西变成了自己的依赖"
	case KindTip:
		return "塔尖只许依赖五站、横切包、契约与密钥管理包——它的薄厚看依赖清单，不看行数"
	}
	return "这一行不在规则表允许的依赖范围内"
}

func depList(deps []string) string {
	if len(deps) == 0 {
		return "标准库（不依赖工程里任何包）"
	}
	return "标准库与 " + fmt.Sprintf("%v", deps)
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
