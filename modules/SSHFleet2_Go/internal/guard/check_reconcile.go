package guard

import "fmt"

// reconcile 是包清单对账：规则表多一行、少一行都报。
func reconcile(m *Module) []Violation {
	var out []Violation

	present := map[string]bool{}
	for _, p := range m.Packages {
		if p.Rule == nil {
			out = append(out, violationAt(ClassDeps, p.Path,
				fmt.Sprintf("包 %s 不在规则表里（规则表二十行，没有它）", p.Path),
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
