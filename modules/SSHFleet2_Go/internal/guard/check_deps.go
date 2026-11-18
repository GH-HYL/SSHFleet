package guard

// CheckDeps 是类二：谁 import 谁。
//
// 三档（ADR-0032 第四条）：生产文件按规则表那一行；普通 _test.go 豁免；红线测试不享这条豁免，
// 按它自己的 import 白名单判（那一条在类六）。另含四条窄判据：塔尖的点名符号、模块之外的
// import、谁许读当前时刻（checkClock）、不经 import 的边（checkLinkEdges）。
//
// 规则表与真实包清单对不上也归这一类：「多一个」「少一个」都是这张尺子失准，不另立类号
// （map.md 阶段表 01 那一行）。
//
// 每条判据一个文件：`check_imports.go`（import 边与模块外的白名单）、`check_reconcile.go`
// （包清单对账）、`check_tip_symbols.go`、`check_clock.go`、`check_link_edges.go`。
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
	out = append(out, checkLinkEdges(m)...)
	return append(out, checkClock(m)...)
}
